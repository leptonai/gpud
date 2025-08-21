package asn

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

const asLookupMaxRetries = 3

type ASLookupResponse struct {
	Asn      string `json:"asn"`
	AsnName  string `json:"asn_name"`
	AsnRange string `json:"asn_range"`
	Country  string `json:"country"`
	IP       string `json:"ip"`
}

func GetASLookup(ip string) (*ASLookupResponse, error) {
	var lastErr error

	for attempt := 1; attempt <= asLookupMaxRetries; attempt++ {
		resp, err := fetchASLookup(ip)

		// Case 1: Error is returned
		if err != nil {
			lastErr = err
			log.Logger.Warnw("ASN lookup attempt failed", "attempt", attempt, "error", err)
			if attempt < asLookupMaxRetries {
				time.Sleep(3 * time.Second)
				continue
			}
			return nil, lastErr
		}

		// Case 2: Response is not nil but AsnName is empty
		if resp != nil && resp.AsnName == "" {
			log.Logger.Warnw("ASN lookup returned empty ASN name, retrying", "attempt", attempt, "ip", ip)
			if attempt < asLookupMaxRetries {
				time.Sleep(3 * time.Second)
				continue
			}
			// Return the response even if AsnName is empty after all retries
			return resp, nil
		}

		// Success - return immediately
		return resp, nil
	}

	return nil, lastErr
}

func fetchASLookup(ip string) (*ASLookupResponse, error) {
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
	"oracle":  "oci",     // e.g., "oracle-bmc-31898" should be "oci"
}
