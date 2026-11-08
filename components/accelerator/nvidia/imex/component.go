package imex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

const Name = "accelerator-nvidia-imex"

type component struct {
	ctx      context.Context
	cancel   context.CancelFunc
	config   common.IMEXConfig
	required bool
	run      func(context.Context, []string) (string, error)
	checkMu  sync.Mutex
	mu       sync.RWMutex
	last     *result
}

func New(instance *components.GPUdInstance) (components.Component, error) {
	ctx, cancel := context.WithCancel(instance.RootCtx)
	c := &component{ctx: ctx, cancel: cancel, config: instance.NVIDIAToolOverwrites.IMEX, run: runCommand}
	if instance.NVMLInstance != nil {
		c.required = product.UsesExternalNVLinkFabric(instance.NVMLInstance.ProductName())
	}
	if c.config.Required != nil {
		c.required = *c.config.Required
	}
	if len(c.config.Command) == 0 {
		c.config.Command = []string{"nvidia-imex-ctl"}
	}
	if len(c.config.ServiceCommand) == 0 {
		c.config.ServiceCommand = []string{"systemctl", "is-active", "nvidia-imex"}
	}
	if c.config.ConfigFile == "" {
		c.config.ConfigFile = "/etc/nvidia-imex/config.cfg"
	}
	return c, nil
}
func (*component) Name() string        { return Name }
func (*component) Tags() []string      { return []string{"accelerator", "gpu", "nvidia", Name} }
func (c *component) IsSupported() bool { return c.required }
func (c *component) Start() error {
	go func() {
		for {
			c.Check()
			// Avoid every tray querying the whole domain simultaneously.
			timer := time.NewTimer(2*time.Minute + time.Duration(rand.IntN(30))*time.Second)
			select {
			case <-c.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return nil
}
func (c *component) Close() error                                          { c.cancel(); return nil }
func (*component) Events(context.Context, time.Time) (apiv1.Events, error) { return nil, nil }
func (c *component) LastHealthStates() apiv1.HealthStates {
	c.mu.RLock()
	r := c.last
	c.mu.RUnlock()
	return r.HealthStates()
}

func (c *component) Check() components.CheckResult {
	c.checkMu.Lock()
	defer c.checkMu.Unlock()
	r := &result{state: apiv1.HealthState{Time: metav1.Now(), Component: Name, Name: Name, Health: apiv1.HealthStateTypeHealthy, Evaluation: apiv1.Evaluated(), FaultScope: apiv1.FaultScopeIMEXDomain}}
	defer func() { c.mu.Lock(); c.last = r; c.mu.Unlock() }()
	if !c.required {
		r.state.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedUnsupported)
		r.state.Reason = "IMEX is not required by this deployment"
		return r
	}
	ctx, cancel := context.WithTimeout(c.ctx, 20*time.Second)
	defer cancel()
	service, serviceErr := c.run(ctx, c.config.ServiceCommand)
	args := append(append([]string{}, c.config.Command...), "-c", c.config.ConfigFile, "-N")
	output, err := c.run(ctx, args)
	if err == nil {
		r.snapshot, err = ParseSnapshot(output)
	}
	if err != nil {
		r.state.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedCollection)
		r.state.Health = apiv1.HealthStateTypeDegraded
		r.state.Reason = "IMEX connectivity evidence unavailable"
		r.state.Error = err.Error()
	}
	var failures []string
	// A missing command service does not prove the daemon has stopped.
	if serviceErr != nil && (strings.TrimSpace(service) == "inactive" || strings.TrimSpace(service) == "failed") {
		failures = append(failures, "local IMEX daemon is "+strings.TrimSpace(service))
	}
	if s := r.snapshot; s != nil {
		if s.Daemon != "READY" {
			failures = append(failures, "local IMEX daemon reports "+s.Daemon)
		}
		for _, p := range s.Peers {
			if p.Status == "connected" && p.Daemon != "READY" {
				failures = append(failures, fmt.Sprintf("peer %d (%s) daemon reports %s", p.NodeID, p.Address, p.Daemon))
			}
			if p.Status != "connected" {
				failures = append(failures, fmt.Sprintf("peer %d (%s): %s", p.NodeID, p.Address, p.Status))
			}
		}
		for observer, row := range s.Matrix {
			for peer, status := range row {
				if status == "unavailable" {
					r.state.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedPartial)
				}
				if observer != s.NodeID && observer != peer && status != "connected" {
					failures = append(failures, fmt.Sprintf("observer %d to peer %d: %s", observer, peer, status))
				}
			}
		}
		// Retain remote-only failures too; never infer a physical culprit here.
		if s.DomainState != "UP" && len(failures) == 0 {
			failures = append(failures, "IMEX domain reports "+s.DomainState)
		}
		if len(c.config.ExpectedAddresses) > 0 {
			observed := make(map[string]bool)
			for _, p := range s.Peers {
				observed[p.Address] = true
			}
			// Include the local node by reading its address from the same snapshot.
			for _, line := range strings.Split(output, "\n") {
				if m := nodeLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil && m[2] == "*" {
					observed[m[3]] = true
				}
			}
			if len(observed) != len(c.config.ExpectedAddresses) {
				failures = append(failures, "IMEX domain membership count differs from inventory")
			}
			for _, address := range c.config.ExpectedAddresses {
				if !observed[address] {
					failures = append(failures, "configured peer missing: "+address)
				}
			}
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		r.state.Health = apiv1.HealthStateTypeUnhealthy
		r.state.Reason = strings.Join(failures, "; ")
		r.state.SuggestedActions = &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeCheckSharedFabric}}
	} else if err == nil {
		r.state.Reason = "IMEX daemon ready and all configured peers connected"
	}
	return r
}

type result struct {
	state    apiv1.HealthState
	snapshot *Snapshot
}

func (*result) ComponentName() string { return Name }
func (r *result) String() string      { return r.Summary() }
func (r *result) Summary() string {
	if r == nil {
		return "no data yet"
	}
	return r.state.Reason
}
func (r *result) HealthStateType() apiv1.HealthStateType {
	if r == nil {
		return apiv1.HealthStateTypeHealthy
	}
	return r.state.Health
}
func (r *result) HealthStates() apiv1.HealthStates {
	if r == nil {
		return apiv1.HealthStates{{Component: Name, Name: Name, Health: apiv1.HealthStateTypeHealthy, Reason: "no data yet", Evaluation: apiv1.NotEvaluated(apiv1.NotEvaluatedNoData)}}
	}
	s := r.state
	if r.snapshot != nil {
		b, _ := json.Marshal(r.snapshot)
		s.ExtraInfo = map[string]string{"imex": string(b)}
	}
	return apiv1.HealthStates{s}
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 2<<20 {
		return 0, fmt.Errorf("IMEX output exceeds 2 MiB")
	}
	return b.Buffer.Write(p)
}
func runCommand(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var output boundedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	return output.String(), err
}
