// "sync" syncs the tailscale derp map.
package main

import (
	"fmt"

	_ "embed"

	"github.com/leptonai/gpud/components/network/latency/derpmap"
)

const derpmapPath = "../derpmap.json"

// sync reads the DERP map from the tailscale public DERP map and writes the data locally to derpmapPath
func main() {
	d, err := derpmap.GetTailcaleDERPMap()
	if err != nil {
		fmt.Printf("failed to get DERP map: %v", err)
	}

	err = d.WriteJSON(derpmapPath)
	if err != nil {
		fmt.Printf("failed to write DERP map: %v", err)
	}
}
