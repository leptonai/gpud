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
