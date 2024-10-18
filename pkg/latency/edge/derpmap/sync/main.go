// "sync" syncs the tailscale derp map.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/leptonai/gpud/pkg/latency/edge/derpmap"
)

const derpmapPath = "../derpmap.json"

// sync reads the DERP map from the tailscale public DERP map and writes the data locally to derpmapPath
func main() {
	d, err := derpmap.DownloadTailcaleDERPMap()
	if err != nil {
		fmt.Printf("failed to get DERP map: %v", err)
	}

	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		fmt.Printf("failed to marshal DERP map: %v", err)
	}

	if err = os.WriteFile(derpmapPath, data, 0o644); err != nil {
		fmt.Printf("failed to write DERP map: %v", err)
	}
}
