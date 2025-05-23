package asn

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ASLookupResponse struct {
	Asn      string `json:"asn"`
	AsnName  string `json:"asn_name"`
	AsnRange string `json:"asn_range"`
	Country  string `json:"country"`
	IP       string `json:"ip"`
}

func GetASLookup(ip string) (*ASLookupResponse, error) {
	url := fmt.Sprintf("https://api.hackertarget.com/aslookup/?q=%s&output=json", ip)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result ASLookupResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}
	asnResults := strings.Split(result.AsnName, ",")
	result.AsnName = strings.ToLower(strings.TrimSpace(asnResults[0]))
	if len(asnResults) > 1 {
		result.Country = strings.ToLower(strings.TrimSpace(asnResults[1]))
	}
	return &result, nil
}

func NormalizeASNName(asnName string) string {
	asnName = strings.TrimSpace(asnName)
	asnName = strings.ToLower(asnName)
	for keyword, normalizedName := range providerKeywords {
		if strings.Contains(asnName, keyword) {
			return normalizedName
		}
	}
	return asnName
}

// providerKeywords is a map of provider keywords to their normalized names.
// It is used to map provider names to their normalized names.
var providerKeywords = map[string]string{
	"aws":     "aws",
	"azure":   "azure",
	"gcp":     "gcp",
	"google":  "gcp",
	"yotta":   "yotta",
	"nebius":  "nebius",  // e.g., "nebiuscloud" should be "nebius"
	"hetzner": "hetzner", // e.g., "hetzner-cloud3-as" should be "hetzner"
}
