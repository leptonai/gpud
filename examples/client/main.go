package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/leptonai/gpud/api/v1"
)

func fetchComponentInfo(baseURL string, componentName string) (v1.LeptonInfo, error) {
	queryURL, err := url.Parse(fmt.Sprintf("%s/v1/info", baseURL))
	if err != nil {
		return nil, err
	}

	q := queryURL.Query()
	if componentName != "" {
		q.Add("component", componentName)
	}
	queryURL.RawQuery = q.Encode()

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := httpClient.Get(queryURL.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var info v1.LeptonInfo
	err = json.Unmarshal(body, &info)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func main() {
	baseURL := "https://localhost:15132"
	componentName := "" // Leave empty to query all components

	info, err := fetchComponentInfo(baseURL, componentName)
	if err != nil {
		fmt.Println("Error fetching component info:", err)
		return
	}

	fmt.Println("Component Information:")
	for _, i := range info {
		fmt.Printf("Component: %s\n", i.Component)
		for _, event := range i.Info.Events {
			fmt.Printf("  Event: %s - %s\n", event.Name, event.Message)
		}
		for _, metric := range i.Info.Metrics {
			fmt.Printf("  Metric: %s - Value: %f\n", metric.MetricName, metric.Value)
		}
		for _, state := range i.Info.States {
			fmt.Printf("  State: %s - Healthy: %t\n", state.Name, state.Healthy)
		}
	}
}
