package asn

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
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

// lookupPrimary and lookupFallback allow tests to swap out the network-dependent
// implementations. They default to the real fetch functions.
var (
	lookupPrimary  = fetchASLookupHackerTarget
	lookupFallback = fetchASLookupTeamCymru
)

func GetASLookup(ip string) (*ASLookupResponse, error) {
	var lastErr error

	for attempt := 1; attempt <= asLookupMaxRetries; attempt++ {
		resp, err := lookupPrimary(ip)

		// Case 1: Error is returned
		if err != nil {
			fallbackResp, fallbackErr := lookupFallback(ip)
			if fallbackErr == nil {
				if fallbackResp != nil && fallbackResp.AsnName == "" {
					log.Logger.Warnw("ASN fallback returned empty ASN name", "attempt", attempt, "ip", ip)
				}
				log.Logger.Infow("ASN lookup succeeded via fallback", "attempt", attempt, "ip", ip)
				return fallbackResp, nil
			}

			lastErr = fmt.Errorf("hackertarget lookup failed: %w; fallback lookup failed: %v", err, fallbackErr)
			log.Logger.Warnw("ASN lookup attempt failed", "attempt", attempt, "ip", ip, "error", lastErr)
			if attempt < asLookupMaxRetries {
				time.Sleep(3 * time.Second)
				continue
			}
			return nil, lastErr
		}

		// Case 2: Response is not nil but AsnName is empty
		if resp != nil && resp.AsnName == "" {
			fallbackResp, fallbackErr := lookupFallback(ip)
			if fallbackErr == nil && fallbackResp != nil && fallbackResp.AsnName != "" {
				log.Logger.Infow("ASN lookup populated via fallback after empty primary response", "attempt", attempt, "ip", ip)
				return fallbackResp, nil
			}

			log.Logger.Warnw("ASN lookup returned empty ASN name, retrying", "attempt", attempt, "ip", ip, "fallback_error", fallbackErr)
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

// fetchASLookupHackerTarget queries the HackerTarget API for ASN information
func fetchASLookupHackerTarget(ip string) (*ASLookupResponse, error) {
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
	result.AsnName, result.Country = sanitizeASNNameAndCountry(result.AsnName, result.Country)
	return &result, nil
}

// fetchASLookupTeamCymru queries the Team Cymru DNS API for ASN information
func fetchASLookupTeamCymru(ip string) (*ASLookupResponse, error) {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ip)
	}

	originDomain, err := teamCymruOriginDomain(parsedIP)
	if err != nil {
		return nil, err
	}

	originRecords, err := net.LookupTXT(originDomain)
	if err != nil {
		return nil, err
	}
	if len(originRecords) == 0 {
		return nil, fmt.Errorf("no TXT records found for %s", originDomain)
	}

	originFields := parseTeamCymruRecord(originRecords[0])
	if len(originFields) < 2 {
		return nil, fmt.Errorf("unexpected response format for %s: %q", originDomain, originRecords[0])
	}

	lookup := &ASLookupResponse{
		Asn:      originFields[0],
		AsnRange: originFields[1],
		Country:  strings.ToLower(originFieldsValue(originFields, 2)),
		IP:       ip,
	}

	asnDetailsDomain := fmt.Sprintf("AS%s.asn.cymru.com", lookup.Asn)
	detailsRecords, err := net.LookupTXT(asnDetailsDomain)
	if err != nil {
		return nil, err
	}
	if len(detailsRecords) == 0 {
		return nil, fmt.Errorf("no TXT records found for %s", asnDetailsDomain)
	}

	detailsFields := parseTeamCymruRecord(detailsRecords[0])
	if len(detailsFields) < 2 {
		return nil, fmt.Errorf("unexpected response format for %s: %q", asnDetailsDomain, detailsRecords[0])
	}

	// ASN details format: "ASN | CC | Registry | Allocated | AS Name"
	// Example: "15169 | US | arin | 2000-03-30 | GOOGLE, US"
	if len(detailsFields) >= 5 {
		lookup.AsnName, lookup.Country = sanitizeASNNameAndCountry(detailsFields[4], lookup.Country)
	} else if len(detailsFields) >= 2 {
		// Fallback to old format if different
		lookup.AsnName, lookup.Country = sanitizeASNNameAndCountry(detailsFields[1], lookup.Country)
	}

	if country := originFieldsValue(detailsFields, 1); country != "" {
		lookup.Country = strings.ToLower(country)
	}

	return lookup, nil
}

func sanitizeASNNameAndCountry(rawName string, fallbackCountry string) (string, string) {
	name := strings.TrimSpace(rawName)
	country := strings.ToLower(strings.TrimSpace(fallbackCountry))

	if name == "" {
		return "", country
	}

	parts := strings.Split(name, ",")
	cleanName := strings.ToLower(strings.TrimSpace(parts[0]))
	if len(parts) > 1 {
		candidate := strings.TrimSpace(parts[1])
		if candidate != "" {
			country = strings.ToLower(candidate)
		}
	}

	return cleanName, country
}

// FetchASLookupHackerTarget exposes the HackerTarget lookup for integration tests.
func FetchASLookupHackerTarget(ip string) (*ASLookupResponse, error) {
	return fetchASLookupHackerTarget(ip)
}

// FetchASLookupTeamCymru exposes the Team Cymru lookup for integration tests.
func FetchASLookupTeamCymru(ip string) (*ASLookupResponse, error) {
	return fetchASLookupTeamCymru(ip)
}

func teamCymruOriginDomain(ip net.IP) (string, error) {
	if ipv4 := ip.To4(); ipv4 != nil {
		return fmt.Sprintf("%d.%d.%d.%d.origin.asn.cymru.com", ipv4[3], ipv4[2], ipv4[1], ipv4[0]), nil
	}

	ipv6 := ip.To16()
	if ipv6 == nil {
		return "", fmt.Errorf("invalid IP address")
	}

	hexDigits := hex.EncodeToString(ipv6)
	reversed := make([]string, len(hexDigits))
	for i := len(hexDigits) - 1; i >= 0; i-- {
		reversed[len(hexDigits)-1-i] = string(hexDigits[i])
	}

	return strings.Join(reversed, ".") + ".origin6.asn.cymru.com", nil
}

func parseTeamCymruRecord(record string) []string {
	fields := strings.Split(record, "|")
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return fields
}

func originFieldsValue(fields []string, idx int) string {
	if idx >= len(fields) {
		return ""
	}
	return fields[idx]
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
