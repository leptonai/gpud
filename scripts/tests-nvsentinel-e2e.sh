#!/usr/bin/env bash
# End-to-end test of the NVSentinel gRPC sink integration using the REAL
# NVSentinel platform-connector (github.com/NVIDIA/nvsentinel), not a
# simulated client.
#
# Chain under test:
#   sender (NVSentinel's own generated client stubs)
#     -> platform-connector gRPC server  (real NVSentinel code)
#     -> pipeline + ring buffer          (real NVSentinel code)
#     -> gRPC sink connector             (real NVSentinel code)
#     -> gpud nvsentinel receiver socket (this repo, pkg/nvsentinel)
#
# Requires: go (toolchain for NVSentinel downloads automatically), git.
# Runs on Linux and macOS; no GPU, NVML, Kubernetes, or NATS needed.
set -euo pipefail

cd "$(dirname "$0")/.."
REPO_ROOT="$(pwd)"

# NVSentinel revision pinned for reproducibility (main @ 2026-08-21).
NVSENTINEL_REF="7fe629ceb2c31baf1c14658182c608b9eb09cb0d"

WORK="$(mktemp -d /tmp/nvsentinel-e2e.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT

echo "==> building gpud receiver (this repo)"
go build -o "$WORK/nvs-recv" ./cmd/nvsentinel-integration-test/

echo "==> cloning NVSentinel @ $NVSENTINEL_REF"
git clone --quiet --filter=blob:none https://github.com/NVIDIA/nvsentinel.git "$WORK/nvsentinel"
git -C "$WORK/nvsentinel" checkout --quiet "$NVSENTINEL_REF"

echo "==> building the real NVSentinel platform-connector"
(cd "$WORK/nvsentinel/platform-connectors" && go build -o "$WORK/platform-connectors" .)

echo "==> writing the sender (uses NVSentinel's own generated stubs)"
mkdir -p "$WORK/sender"
cat > "$WORK/sender/go.mod" <<EOF
module nvse2esender

go 1.25.0

require (
	github.com/nvidia/nvsentinel/data-models v0.0.0
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.12
)

replace github.com/nvidia/nvsentinel/data-models => $WORK/nvsentinel/data-models
EOF
cat > "$WORK/sender/main.go" <<'EOF'
// Command sender publishes one health event to the real NVSentinel
// platform-connector, the same RPC a health monitor makes.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/nvidia/nvsentinel/data-models/pkg/protos"
)

func main() {
	conn, err := grpc.NewClient("unix://"+os.Args[1], grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAIL: dial:", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	event := &pb.HealthEvent{
		Version:            1,
		Agent:              "syslog-health-monitor",
		ComponentClass:     "GPU",
		CheckName:          "SysLogsXIDError",
		IsFatal:            true,
		Message:            "Xid 79: GPU has fallen off the bus",
		RecommendedAction:  pb.RecommendedAction_RESTART_BM,
		ErrorCode:          []string{"79"},
		EntitiesImpacted:   []*pb.Entity{{EntityType: "GPU", EntityValue: "GPU-e2e-0001"}},
		GeneratedTimestamp: timestamppb.Now(),
		NodeName:           "e2e-node",
		Id:                 "e2e-event-1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = pb.NewPlatformConnectorClient(conn).HealthEventOccurredV1(ctx, &pb.HealthEvents{Version: 1, Events: []*pb.HealthEvent{event}})
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAIL: HealthEventOccurredV1:", err)
		os.Exit(1)
	}
	fmt.Println("SEND-OK")
}
EOF
(cd "$WORK/sender" && go mod tidy >/dev/null && go build -o "$WORK/nvs-send" .)

cat > "$WORK/pc-config.json" <<EOF
{
  "enableGRPCSinkConnector": "true",
  "GRPCSinkTarget": "unix://$WORK/gpud.sock",
  "GRPCSinkConnectorMaxRetries": 3,
  "enableNodeBindingAuth": false,
  "pipeline": [{"name": "dedup", "enabled": false, "config": ""}]
}
EOF

echo "==> starting gpud nvsentinel receiver (recv mode)"
"$WORK/nvs-recv" -mode recv -socket "$WORK/gpud.sock" > "$WORK/recv.log" 2>&1 &
RECV_PID=$!
for _ in $(seq 1 100); do [ -S "$WORK/gpud.sock" ] && break; sleep 0.1; done
[ -S "$WORK/gpud.sock" ] || { echo "FAIL: receiver socket never appeared"; cat "$WORK/recv.log"; exit 1; }

echo "==> starting the real NVSentinel platform-connector"
"$WORK/platform-connectors" -socket "$WORK/pc.sock" -config "$WORK/pc-config.json" -metrics-port 22997 > "$WORK/pc.log" 2>&1 &
PC_PID=$!
for _ in $(seq 1 100); do [ -S "$WORK/pc.sock" ] && break; sleep 0.1; done
[ -S "$WORK/pc.sock" ] || { echo "FAIL: platform-connector socket never appeared"; cat "$WORK/pc.log"; exit 1; }

echo "==> publishing health event through the real platform-connector"
"$WORK/nvs-send" "$WORK/pc.sock"

echo "==> waiting for the receiver verdict"
STATUS=0
wait $RECV_PID || STATUS=$?
kill $PC_PID 2>/dev/null || true

cat "$WORK/recv.log"
if [ "$STATUS" -ne 0 ]; then
  echo "FAIL: receiver exited with $STATUS"
  echo "--- platform-connector log ---"
  tail -30 "$WORK/pc.log"
  exit "$STATUS"
fi
echo "E2E PASS: real NVSentinel platform-connector delivered the event to gpud"
