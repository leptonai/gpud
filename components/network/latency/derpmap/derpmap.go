// Package derpmap provides the tailscale derp map implementation.
package derpmap

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const (
	TailscaleDERPMapURL = "https://login.tailscale.com/derpmap/default"
	TailescaleRegionKey = "Regions" // key containing the RegionInfo
)

// DERPMap is a map of RegionID to Region
type DERPMap map[int]Region
type Region struct {
	RegionID   int    `json:"RegionID"`
	RegionCode string `json:"RegionCode"`
	RegionName string `json:"RegionName"`
	Nodes      []Node `json:"Nodes"`
}

// Node stores node information pulled from the tailscale public DERP map
type Node struct {
	Name     string `json:"Name"`
	RegionID int    `json:"RegionID"`
	HostName string `json:"HostName"`
	IPv4     string `json:"IPv4"`
	IPv6     string `json:"IPv6"`
}

var SavedDERPMap DERPMap // SavedDERPMap is the DERP map that is embedded from derpmap.json at compile time

//go:embed derpmap.json
var DERPMapRaw embed.FS

func init() {
	data, _ := DERPMapRaw.ReadFile("derpmap.json")
	err := json.Unmarshal(data, &SavedDERPMap)
	if err != nil {
		fmt.Printf("failed to load DERP map: %v", err)
	}
}

// GetTailscaleDERPMap fetches the Tailscale public DERP map
// and loads it into a DERPMap struct
func GetTailcaleDERPMap() (DERPMap, error) {
	var data map[string]DERPMap
	res, err := http.Get(TailscaleDERPMapURL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	for k, v := range data {
		if k == TailescaleRegionKey {
			DERPMap := v
			return DERPMap, nil
		}
	}
	return nil, fmt.Errorf("GetTailscaleDERPMap(): unable to load regions")
}

// WriteJSON writes the DERPMap to a JSON file at the given path
func (d DERPMap) WriteJSON(path string) error {
	data, err := json.MarshalIndent(d, "", "    ")
	if err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", path, err)
	}
	defer file.Close()
	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write data to %s: %v", path, err)
	}
	return nil
}

// Generates a regionCode: Region mapping for the DERPMap
func (d DERPMap) GetRegionCodeMapping() map[string]Region {
	codeMap := make(map[string]Region)
	for _, region := range d {
		codeMap[region.RegionCode] = region
	}
	return codeMap
}
