// Package datamodels holds a wire-compatible subset of the
// NVSentinel health_event.proto for receiving PlatformConnector events.
//
//go:generate protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative health_event.proto
package datamodels
