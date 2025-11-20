package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/config"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
	"github.com/leptonai/gpud/pkg/log"
)

const (
	DefaultQuerySince     = 30 * time.Minute
	initializeGracePeriod = 5 * time.Minute
)

// Request is the request from the control plane to GPUd.
type Request struct {
	Method        string            `json:"method,omitempty"`
	Components    []string          `json:"components,omitempty"`
	StartTime     time.Time         `json:"start_time"`
	EndTime       time.Time         `json:"end_time"`
	Since         time.Duration     `json:"since"`
	UpdateVersion string            `json:"update_version,omitempty"`
	UpdateConfig  map[string]string `json:"update_config,omitempty"`

	Bootstrap          *BootstrapRequest         `json:"bootstrap,omitempty"`
	InjectFaultRequest *pkgfaultinjector.Request `json:"inject_fault_request,omitempty"`

	// ComponentName is the name of the component to query or deregister.
	ComponentName string `json:"component_name,omitempty"`

	// TagName is the tag of the component to trigger check.
	// Optional. If set, it triggers all the component checks
	// that match this tag value.
	TagName string `json:"tag_name,omitempty"`

	// CustomPluginSpecs is the specs for the custom plugins to register or overwrite.
	CustomPluginSpecs pkgcustomplugins.Specs `json:"custom_plugin_specs,omitempty"`
}

// Response is the response from GPUd to the control plane.
type Response struct {
	// Error is the error message from session processor.
	// don't use "error" type as it doesn't marshal/unmarshal well
	Error string `json:"error,omitempty"`
	// ErrorCode is the error code from session processor.
	// It uses the same semantics as the HTTP status code.
	// See: https://www.iana.org/assignments/http-status-codes/http-status-codes.xhtml
	ErrorCode int32 `json:"error_code,omitempty"`

	GossipRequest *apiv1.GossipRequest `json:"gossip_request,omitempty"`

	States  apiv1.GPUdComponentHealthStates `json:"states,omitempty"`
	Events  apiv1.GPUdComponentEvents       `json:"events,omitempty"`
	Metrics apiv1.GPUdComponentMetrics      `json:"metrics,omitempty"`

	Bootstrap *BootstrapResponse `json:"bootstrap,omitempty"`

	PackageStatus []apiv1.PackageStatus `json:"package_status,omitempty"`

	// CustomPluginSpecs lists the specs for the custom plugins.
	CustomPluginSpecs pkgcustomplugins.Specs `json:"custom_plugin_specs,omitempty"`
}

type BootstrapRequest struct {
	// TimeoutInSeconds is the timeout for the bootstrap script.
	// If not set, the default timeout is 10 seconds.
	TimeoutInSeconds int `json:"timeout_in_seconds,omitempty"`

	// ScriptBase64 is the base64 encoded script to run.
	ScriptBase64 string `json:"script_base64,omitempty"`
}

type BootstrapResponse struct {
	Output   string `json:"output,omitempty"`
	ExitCode int32  `json:"exit_code,omitempty"`
}

func (s *Session) serve() {
	for body := range s.reader {
		var payload Request
		if err := json.Unmarshal(body.Data, &payload); err != nil {
			log.Logger.Errorw("failed to unmarshal request", "error", err, "requestID", body.ReqID)
			continue
		}

		s.auditLogger.Log(
			log.WithKind("Session"),
			log.WithAuditID(body.ReqID),
			log.WithMachineID(s.machineID),
			log.WithStage("RequestDecoded"),
			log.WithRequestURI(s.epControlPlane+"/api/v1/session"),
			log.WithVerb(payload.Method),
			log.WithData(payload),
		)

		restartExitCode := -1
		response := &Response{}

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		handledAsync := s.processRequest(ctx, body.ReqID, payload, response, &restartExitCode)
		cancel()

		if handledAsync {
			continue
		}

		s.sendResponse(body.ReqID, payload.Method, response)

		if restartExitCode != -1 {
			go func() {
				log.Logger.Warnw("process exiting as scheduled", "code", restartExitCode)
				select {
				case <-s.ctx.Done():
				case <-time.After(10 * time.Second):
					// enough time to send response back to control plane
				}

				s.auditLogger.Log(
					log.WithKind("AutoUpdateRestart"),
					log.WithAuditID(body.ReqID),
					log.WithMachineID(s.machineID),
				)
				os.Exit(s.autoUpdateExitCode)
			}()
		}
	}
}

func (s *Session) delete() {
	// cleanup packages
	if err := createNeedDeleteFiles(config.PackagesDir(s.dataDir)); err != nil {
		log.Logger.Errorw("failed to delete packages",
			"error", err,
		)
	}
}

func createNeedDeleteFiles(rootPath string) error {
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	return filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && path != rootPath {
			needDeleteFilePath := filepath.Join(path, "needDelete")
			file, err := os.Create(needDeleteFilePath)
			if err != nil {
				return fmt.Errorf("failed to create needDelete file in %s: %w", path, err)
			}
			defer file.Close()
		}
		return nil
	})
}
