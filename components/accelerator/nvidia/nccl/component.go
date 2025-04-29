// Package nccl monitors the NCCL status.
// Optional, enabled if the host has NVIDIA GPUs.
package nccl

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const Name = "accelerator-nvidia-nccl"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.Instance

	eventBucket eventstore.Bucket
	kmsgSyncer  *kmsg.Syncer

	readAllKmsg func(context.Context) ([]kmsg.Message, error)

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: gpudInstance.NVMLInstance,
	}

	if gpudInstance.EventStore != nil && runtime.GOOS == "linux" {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}

		if os.Geteuid() == 0 {
			c.kmsgSyncer, err = kmsg.NewSyncer(cctx, Match, c.eventBucket)
			if err != nil {
				ccancel()
				return nil, err
			}
		}
	}

	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		c.readAllKmsg = kmsg.ReadAll
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	// do not need periodic kmsg checks since it already has a watcher
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	return apiv1.HealthStates{
		{
			Component: Name,
			Health:    apiv1.HealthStateTypeHealthy,
			Reason:    "no issue",
		},
	}
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if c.eventBucket == nil {
		return nil, nil
	}
	return c.eventBucket.Get(ctx, since)
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	if c.kmsgSyncer != nil {
		c.kmsgSyncer.Close()
	}
	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu nccl")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML instance is nil"
		return cr
	}
	if !c.nvmlInstance.NVMLExists() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML library is not loaded"
		return cr
	}
	if c.nvmlInstance.ProductName() == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML is loaded but GPU is not detected (missing product name)"
		return cr
	}

	if c.readAllKmsg == nil {
		cr.reason = "kmsg reader is not set"
		cr.health = apiv1.HealthStateTypeHealthy
		return cr
	}

	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	kmsgs, err := c.readAllKmsg(cctx)
	ccancel()
	if err != nil {
		cr.err = err
		cr.reason = "failed to read kmsg"
		cr.health = apiv1.HealthStateTypeUnhealthy
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr
	}

	for _, kmsg := range kmsgs {
		ev, _ := Match(kmsg.Message)
		if ev == "" {
			continue
		}
		cr.MatchedKmsgs = append(cr.MatchedKmsgs, kmsg)
	}

	cr.reason = "scanned kmsg(s)"
	cr.health = apiv1.HealthStateTypeHealthy

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	MatchedKmsgs []kmsg.Message `json:"matched_kmsgs"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	return fmt.Sprintf("matched %d kmsg(s)", len(cr.MatchedKmsgs))
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) getError() string {
	if cr == nil || cr.err == nil {
		return ""
	}
	return cr.err.Error()
}

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Time:          metav1.NewTime(time.Now().UTC()),
				Component:     Name,
				ComponentType: apiv1.ComponentTypeComponent,
				Name:          Name,
				Health:        apiv1.HealthStateTypeHealthy,
				Reason:        "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:          metav1.NewTime(cr.ts),
		Component:     Name,
		ComponentType: apiv1.ComponentTypeComponent,
		Name:          Name,
		Reason:        cr.reason,
		Error:         cr.getError(),
		Health:        cr.health,
	}
	return apiv1.HealthStates{state}
}
