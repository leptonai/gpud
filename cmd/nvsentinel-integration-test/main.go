// Command nvsentinel-integration-test verifies the NVSentinel integration
// end-to-end without requiring NVML or real GPU hardware. It starts the
// gRPC receiver, sends a simulated health event, and checks that the
// event flows through the subscriber pipeline.
//
// Build and run in a Linux container:
//
//	podman build -f cmd/nvsentinel-integration-test/Containerfile -t nvsentinel-test .
//	podman run --rm nvsentinel-test
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvsentinel"
	"github.com/leptonai/gpud/pkg/nvsentinel/proto/datamodels"
)

var (
	mode       = flag.String("mode", "selftest", "selftest runs the full in-process check; recv only listens on -socket and asserts one matching event arrives (used by scripts/tests-nvsentinel-e2e.sh with the real NVSentinel platform-connector)")
	socketPath = flag.String("socket", "", "unix socket path for the receiver (recv mode only)")
)

func main() {
	flag.Parse()

	if *mode == "recv" {
		if *socketPath == "" {
			fmt.Fprintln(os.Stderr, "FAIL: -socket is required in recv mode")
			os.Exit(1)
		}
		if err := recv(*socketPath); err != nil {
			fmt.Fprintln(os.Stderr, "FAIL:", err)
			os.Exit(1)
		}
		fmt.Println("PASS")
		return
	}
	selftest()
}

// recv starts the receiver on socketPath and blocks until one NVSentinel
// event arrives and passes all assertions.
func recv(socketPath string) error {
	src, err := nvsentinel.New(socketPath)
	if err != nil {
		return fmt.Errorf("nvsentinel.New: %w", err)
	}
	defer func() { _ = src.Close() }()

	events, stop := src.Subscribe()
	defer stop()

	log.Logger.Infow("receiver listening", "socket", socketPath)

	select {
	case ev := <-events:
		return assertEvent(ev, src)
	case <-time.After(60 * time.Second):
		return fmt.Errorf("timed out waiting for subscriber event")
	}
}

func selftest() {
	log.Logger.Infow("nvsentinel integration test starting")

	tmpDir, err := os.MkdirTemp("", "nvsentinel-test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: mkdir temp: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	socketPath := filepath.Join(tmpDir, "nvsentinel.sock")
	log.Logger.Infow("using socket", "path", socketPath)

	src, err := nvsentinel.New(socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: nvsentinel.New: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = src.Close() }()

	events, stop := src.Subscribe()
	defer stop()

	// Connect a simulated NVSentinel platform-connector client and send a
	// health event the same way the gRPC sink connector would.
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: grpc.NewClient: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	client := datamodels.NewPlatformConnectorClient(conn)

	sent := &datamodels.HealthEvents{
		Version: 1,
		Events: []*datamodels.HealthEvent{{
			Version:            1,
			Agent:              "syslog-health-monitor",
			ComponentClass:     "GPU",
			CheckName:          "SysLogsXIDError",
			IsFatal:            true,
			Message:            "Xid 79: GPU has fallen off the bus",
			RecommendedAction:  datamodels.RecommendedAction_RESTART_BM,
			ErrorCode:          []string{"79"},
			EntitiesImpacted:   []*datamodels.Entity{{EntityType: "GPU", EntityValue: "GPU-test-001"}},
			GeneratedTimestamp: timestamppb.Now(),
			NodeName:           "test-node",
			Id:                 "integ-test-1",
		}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.HealthEventOccurredV1(ctx, sent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: HealthEventOccurredV1: %v\n", err)
		os.Exit(1)
	}

	log.Logger.Infow("event sent, waiting for subscriber")

	select {
	case ev := <-events:
		if err := assertEvent(ev, src); err != nil {
			fmt.Fprintln(os.Stderr, "FAIL:", err)
			os.Exit(1)
		}
		log.Logger.Infow("subscriber received event correctly", "checkName", ev.CheckName)
	case <-time.After(5 * time.Second):
		fmt.Fprintln(os.Stderr, "FAIL: timed out waiting for subscriber event")
		os.Exit(1)
	}

	log.Logger.Infow("all checks passed")
	fmt.Println("PASS")
}

// assertEvent checks the event the e2e harness and selftest both send:
// a fatal SysLogsXIDError for Xid 79 on a GPU entity.
func assertEvent(ev nvsentinel.HealthEvent, src nvsentinel.Source) error {
	if ev.CheckName != "SysLogsXIDError" {
		return fmt.Errorf("wrong checkName: %q", ev.CheckName)
	}
	if ev.ComponentClass != "GPU" {
		return fmt.Errorf("wrong componentClass: %q", ev.ComponentClass)
	}
	if !ev.IsFatal {
		return fmt.Errorf("expected IsFatal=true")
	}
	if len(ev.ErrorCodes) != 1 || ev.ErrorCodes[0] != "79" {
		return fmt.Errorf("wrong errorCodes: %v", ev.ErrorCodes)
	}
	if _, ok := ev.EntityValue("GPU"); !ok {
		return fmt.Errorf("missing GPU entity")
	}
	if src.LastReceived().IsZero() {
		return fmt.Errorf("lastReceived is zero")
	}

	// Verify Covers reports the event.
	matchXid79 := func(ev nvsentinel.HealthEvent) bool {
		return ev.CheckName == "SysLogsXIDError" && len(ev.ErrorCodes) > 0 && ev.ErrorCodes[0] == "79"
	}
	if !src.Covers(2*time.Minute, matchXid79) {
		return fmt.Errorf("covers returned false for the event we just received")
	}
	return nil
}
