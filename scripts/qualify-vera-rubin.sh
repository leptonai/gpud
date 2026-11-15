#!/usr/bin/env bash
# Capture one-shot qualification evidence without installing or restarting GPUd.
# Usage: bash scripts/qualify-vera-rubin.sh /absolute/path/gpud NEW_OUTPUT_DIR [scan flags...]
set -uo pipefail

if [[ $# -lt 2 ]]; then
    echo "Usage: $0 GPUD_BINARY NEW_OUTPUT_DIR [scan flags...]" >&2
    exit 2
fi
binary=$1
output_dir=$2
shift 2
if [[ ! -x "$binary" ]]; then
    echo "GPUd binary is not executable: $binary" >&2
    exit 2
fi
for dependency in timeout sha256sum nvidia-smi; do
    if ! command -v "$dependency" >/dev/null 2>&1; then
        echo "Required command missing: $dependency" >&2
        exit 2
    fi
done
# Never mix a new hardware run with an earlier run's files.
mkdir -m 700 -- "$output_dir" || exit 2
printf 'command\texit_code\n' > "$output_dir/exit-codes.tsv"
failed=0
capture() {
    local required=$1 label=$2 status
    shift 2
    printf '%q ' "$@" >> "$output_dir/commands.txt"
    printf '\n' >> "$output_dir/commands.txt"
    # Continue after errors: partial telemetry is useful for driver bringup.
    timeout --signal=TERM --kill-after=10s 300s "$@" > "$output_dir/$label.stdout" 2> "$output_dir/$label.stderr"
    status=$?
    printf '%s\t%d\n' "$label" "$status" >> "$output_dir/exit-codes.tsv"
    if [[ "$required" == required && $status -ne 0 ]]; then
        failed=1
    fi
}
capture required gpud-version "$binary" --version
capture required gpud-sha256 sha256sum "$binary"
capture required uname uname -a
capture required os-release cat /etc/os-release
capture required nvidia-smi nvidia-smi -q
capture optional nvidia-smi-xml nvidia-smi -q -x
capture optional nvlink-state nvidia-smi nvlink -s
capture optional nvlink-p2p nvidia-smi topo -p2p n
capture optional pci lspci -nn
capture optional infiniband ibstat
# A fresh directory avoids reading the installed daemon's login/state database.
capture required machine-info "$binary" machine-info --data-dir "$output_dir/state" --log-level debug
capture required machine-info-json "$binary" machine-info --data-dir "$output_dir/state" --output-format json
capture required scan "$binary" scan --log-level debug "$@"
printf 'Evidence saved to %s. Review component health and N/A fields; scan exit 0 does not certify health.\n' "$output_dir"
exit "$failed"
