package session

import (
	"context"
	"encoding/json"
	"net/http"

	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
)

// processRequest handles all request processing logic
// Returns true if the request is handled asynchronously
// thus no need to wait for its completion to send the
// response back to the control plane
func (s *Session) processRequest(ctx context.Context, reqID string, payload Request, response *Response, restartExitCode *int) bool {
	switch payload.Method {
	case "reboot":
		// To inform the control plane that the reboot request has been processed, reboot after 10 seconds.
		err := pkghost.Reboot(s.ctx, pkghost.WithDelaySeconds(10))
		if err != nil {
			log.Logger.Warnw("failed to trigger reboot machine", "error", err)
			response.Error = err.Error()
		}

	case "metrics":
		metrics, err := s.getMetrics(ctx, payload)
		if err != nil {
			response.Error = err.Error()
		}
		response.Metrics = metrics

	case "states":
		states, err := s.getHealthStates(payload)
		if err != nil {
			response.Error = err.Error()
		}
		response.States = states

	case "events":
		events, err := s.getEvents(ctx, payload)
		if err != nil {
			response.Error = err.Error()
		}
		response.Events = events

	case "delete":
		go s.delete()

	case "logout":
		s.processLogout(ctx, response)

	case "setHealthy":
		s.processSetHealthy(payload)

	case "gossip":
		// Process gossip asynchronously to prevent blocking the session reader loop.
		//
		// Background: Gossip requests collect machine information including disk/filesystem metadata
		// via pkg/machine-info/gossip_request.go. This involves filesystem stat() operations that can
		// hang indefinitely on unresponsive network filesystems (NFS). When stat() enters the
		// kernel, Go cannot interrupt it with context cancellation, causing the goroutine to block
		// in D-state (uninterruptible sleep).
		//
		// If gossip runs synchronously in the session reader loop, a single hung filesystem operation
		// blocks ALL session communication:
		//   1. Reader loop blocks waiting for gossip to complete
		//   2. Reader cannot process other control plane messages
		//   3. Keep-alive mechanism breaks (no heartbeats sent)
		//   4. Control plane marks node as offline/stale
		//
		// With async processing (following the triggerComponent pattern), hung disk operations only
		// affect the specific gossip goroutine, while session communication remains healthy.
		go s.processRequestAsync(reqID, payload.Method, payload)
		return true // Request is handled asynchronously

	case "packageStatus":
		s.processPackageStatus(ctx, response)

	case "update":
		s.processUpdate(ctx, payload, response, restartExitCode)

	case "updateConfig":
		s.processUpdateConfig(payload.UpdateConfig, response)

	case "bootstrap":
		s.processBootstrap(ctx, payload, response)

	case "injectFault":
		s.processInjectFault(payload, response)

		// TODO: deprecate "triggerComponentCheck" after control plane supports "triggerComponent"
	case "triggerComponent",
		"triggerComponentCheck":
		// "triggerComponent" requests can take materially longer than the
		// other control-plane RPCs because each component executes its own `Check()` routine, often
		// hitting external systems (NVML, Kubernetes API, Docker, etc). Previously, the session
		// loop processed these checks synchronously, which meant a single slow component could block
		// the entire `serve()` goroutine and keep subsequent control-plane requests from being read or
		// acknowledged in time. Here we assume that the control plane keys responses using ReqID,
		// thus not rely on strict ordering, so it is safe to emit the reply from a background goroutine
		// as long as the ReqID is preserved. This goroutine launch offloads the expensive `Check()` work
		// and the eventual response write to the background worker so the main session loop stays responsive.
		// Previously, even a single manual trigger aimed at the disk component (whose Check() performs slow
		// disk scans) was enough to wedge the loop: `serve()` blocked on disk.Check(), the control plane timed out
		// and canceled the request, GPUd restarted the session, and the control plane continued using
		// stale health data because no response ever left the box. Running the check asynchronously
		// prevents that deadlock while keeping the previous payload contract intact.
		go s.processRequestAsync(reqID, payload.Method, payload)
		return true // Request is handled asynchronously

	case "deregisterComponent":
		s.processDeregisterComponent(payload, response)

	case "setPluginSpecs":
		exitCode := s.processSetPluginSpecs(ctx, response, payload.CustomPluginSpecs)
		if exitCode != nil {
			*restartExitCode = *exitCode
			log.Logger.Infow("scheduled process exit for plugin specs update", "code", *restartExitCode)
		}

	case "getPluginSpecs":
		s.processGetPluginSpecs(response)
	}

	return false // Request is handled synchronously
}

// processRequestAsync runs entirely in a background goroutine.
// This method handles requests that need to be processed asynchronously to avoid blocking
// the main serve loop. Currently "triggerComponent" and "gossip" requests are processed async
// because they can take significant time (hitting external systems like NVML, Kubernetes API,
// Docker, disk I/O, etc) and may block on unresponsive resources (e.g., stuck NFS mounts).
// The ReqID travels with the request so the control plane can correlate the eventually-delivered response.
func (s *Session) processRequestAsync(reqID, method string, payload Request) {
	response := &Response{}

	switch method {
	case "triggerComponent", "triggerComponentCheck":
		// Process component trigger requests asynchronously
		s.processTriggerComponent(payload, response)

	case "gossip":
		// Process gossip requests asynchronously to avoid blocking on disk I/O operations
		// that may hang if filesystems (especially NFS) are unresponsive
		s.processGossip(response)

	// Add other async method handlers here as needed in the future
	// Note: Only methods that can block for significant time should be handled async

	default:
		// This should not happen as only specific methods are routed here
		log.Logger.Errorw("unsupported async method", "method", method)
		response.Error = "unsupported async method: " + method
		response.ErrorCode = http.StatusBadRequest
	}

	// The asynchronous worker must still emit exactly one response with the original ReqID. sendResponse
	// handles marshaling plus audit logging so replies generated here look identical to the historical
	// synchronous path.
	s.sendResponse(reqID, method, response)
}

// sendResponse centralizes response marshaling, channel delivery, and audit logging. LEP-2083 moved
// triggerComponent replies into a background goroutine, so both synchronous and asynchronous paths now
// route through this helper to keep the control-plane payload contract identical to the pre-PR#1078
// behavior.
func (s *Session) sendResponse(reqID, method string, response *Response) {
	responseRaw, err := json.Marshal(response)
	if err != nil {
		log.Logger.Errorw("session serve: failed to marshal response", "method", method, "reqID", reqID, "error", err)
		return
	}

	body := Body{
		Data:  responseRaw,
		ReqID: reqID,
	}

	if !s.trySendResponse(body) {
		return
	}

	s.auditLogger.Log(
		log.WithKind("Session"),
		log.WithAuditID(reqID),
		log.WithMachineID(s.machineID),
		log.WithStage("RequestCompleted"),
		log.WithRequestURI(s.epControlPlane+"/api/v1/session"),
		log.WithVerb(method),
		log.WithData(response),
	)
}

// trySendResponse attempts to write to s.writer while guarding against shutdown races. When the session
// stops it closes s.writer; without the recover below an in-flight asynchronous write (triggered by
// LEP-2083) would panic and crash the session worker. Returning false lets the caller skip audit logging
// for responses that never made it onto the wire.
func (s *Session) trySendResponse(body Body) (sent bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Logger.Warnw(
				"session serve: failed to write response, writer closed",
				"reqID", body.ReqID,
				"panic", r,
			)
			sent = false
		}
	}()

	select {
	case <-s.ctx.Done():
		log.Logger.Debugw("session serve: dropping response, session context done", "reqID", body.ReqID)
		return false
	case s.writer <- body:
		return true
	}
}
