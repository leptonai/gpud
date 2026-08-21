// Package nvsentinel receives health events from a local NVSentinel
// deployment and exposes them to GPUd components.
//
// NVSentinel's platform-connectors run as a DaemonSet. They serve a
// push-only gRPC API — PlatformConnector.HealthEventOccurredV1 — on a
// unix socket. NVSentinel has no local pull API. The gRPC sink connector
// is its supported egress path: it forwards every health event to a
// configurable target.
//
// This package is the receiver: GPUd serves the same PlatformConnector
// API on its own unix socket, and the node's platform-connector forwards
// events to it. Components prefer matching NVSentinel data points and
// fall back to their own detection when no corresponding NVSentinel data
// point exists.
package nvsentinel
