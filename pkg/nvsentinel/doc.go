// Package nvsentinel receives health events from a local NVSentinel
// deployment and exposes them to GPUd components.
//
// NVSentinel's platform-connectors run as a DaemonSet and serve a push-only
// gRPC API (PlatformConnector.HealthEventOccurredV1) on a unix socket.
// NVSentinel has no local API to pull health data from. Its supported
// egress for a third-party process is the platform-connector "gRPC sink
// connector", which forwards every health event to a configured gRPC target.
//
// This package implements the receiver side of that forwarding: GPUd serves
// the same PlatformConnector API on its own unix socket, and the node's
// platform-connector is configured to forward events to GPUd. Components
// then prefer matching NVSentinel data points and fall back to their own
// detection when no corresponding NVSentinel data point exists.
//
//go:generate protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative -I proto/datamodels proto/datamodels/health_event.proto
package nvsentinel
