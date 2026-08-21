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

func main() {
	log.Logger.Infow("nvsentinel integration test starting")

	tmpDir, err := os.MkdirTemp("", "nvsentinel-test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: mkdir temp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	socketPath := filepath.Join(tmpDir, "nvsentinel.sock")
	log.Logger.Infow("using socket", "path", socketPath)

	src, err := nvsentinel.New(socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: nvsentinel.New: %v\n", err)
		os.Exit(1)
	}
	defer src.Close()

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
	defer conn.Close()

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
		if ev.CheckName != "SysLogsXIDError" {
			fmt.Fprintf(os.Stderr, "FAIL: wrong checkName: %q\n", ev.CheckName)
			os.Exit(1)
		}
		if ev.ComponentClass != "GPU" {
			fmt.Fprintf(os.Stderr, "FAIL: wrong componentClass: %q\n", ev.ComponentClass)
			os.Exit(1)
		}
		if !ev.IsFatal {
			fmt.Fprintf(os.Stderr, "FAIL: expected IsFatal=true\n")
			os.Exit(1)
		}
		if len(ev.ErrorCodes) != 1 || ev.ErrorCodes[0] != "79" {
			fmt.Fprintf(os.Stderr, "FAIL: wrong errorCodes: %v\n", ev.ErrorCodes)
			os.Exit(1)
		}
		if v, ok := ev.EntityValue("GPU"); !ok || v != "GPU-test-001" {
			fmt.Fprintf(os.Stderr, "FAIL: wrong GPU entity: %q (ok=%v)\n", v, ok)
			os.Exit(1)
		}
		log.Logger.Infow("subscriber received event correctly", "checkName", ev.CheckName)
	case <-time.After(5 * time.Second):
		fmt.Fprintf(os.Stderr, "FAIL: timed out waiting for subscriber event\n")
		os.Exit(1)
	}

	// Verify Covers reports the event.
	matchXid79 := func(ev nvsentinel.HealthEvent) bool {
		return ev.CheckName == "SysLogsXIDError" && len(ev.ErrorCodes) > 0 && ev.ErrorCodes[0] == "79"
	}
	if !src.Covers(2*time.Minute, matchXid79) {
		fmt.Fprintf(os.Stderr, "FAIL: Covers returned false for the event we just sent\n")
		os.Exit(1)
	}
	log.Logger.Infow("Covers check passed")

	log.Logger.Infow("all checks passed")
	fmt.Println("PASS")
}
