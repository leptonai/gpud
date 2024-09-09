package e2e_test

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	v1 "github.com/leptonai/gpud/api/v1"
	client_v1 "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/internal/server"
)

func TestGpudHealthzInfo(t *testing.T) {
	if os.Getenv("GPUD_BIN") == "" {
		t.Skip("skipping e2e tests")
	}

	// get an available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to find a free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	ep := fmt.Sprintf("localhost:%d", port)

	randKey := randStr(t, 10)
	randVal := randStr(t, 10)

	// start gpud command
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Getenv("GPUD_BIN"), "run", "--log-level=debug", "--web-enable=false", "--enable-auto-update=false", "--annotations", fmt.Sprintf("{%q:%q}", randKey, randVal), fmt.Sprintf("--listen-address=%s", ep))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start gpud: %v", err)
	}

	defer func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Errorf("failed to kill gpud process: %v", err)
		}
	}()

	// wait for the server to start
	time.Sleep(15 * time.Second)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req1NoCompress, err := http.NewRequest("GET", fmt.Sprintf("https://%s/healthz", ep), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp1, err := client.Do(req1NoCompress)
	if err != nil {
		t.Fatalf("failed to make request to /healthz: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %v", resp1.Status)
	}
	b1, err := io.ReadAll(resp1.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	expectedBody := `{"status":"ok","version":"v1"}`
	if string(b1) != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, string(b1))
	}

	req2NoCompress, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/states", ep), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req2NoCompress.Header.Set("Content-Type", "application/json")
	resp2NoCompress, err := client.Do(req2NoCompress)
	if err != nil {
		t.Errorf("failed to make request to /v1/states: %v", err)
	}
	defer resp2NoCompress.Body.Close()
	if resp2NoCompress.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %v", resp2NoCompress.Status)
	}
	b2NoCompress, err := io.ReadAll(resp2NoCompress.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	t.Logf("resp2NoCompress size: %d", len(b2NoCompress))
	var componentStates []v1.LeptonComponentStates
	if err := json.Unmarshal(b2NoCompress, &componentStates); err != nil {
		t.Errorf("failed to unmarshal states: %v", err)
	}
	found := false
	for _, comp := range componentStates {
		if comp.Component != "info" {
			continue
		}
		for _, state := range comp.States {
			if state.Name != "annotations" {
				continue
			}

			found = true
			if state.ExtraInfo[randKey] != randVal {
				t.Errorf("expected annotation %q=%q, got %q (%s)", randKey, randVal, state.ExtraInfo[randKey], string(b2NoCompress))
			}
		}
	}
	if !found {
		t.Errorf("expected to find annotation state, got %v (%s)", componentStates, string(b2NoCompress))
	}

	req2CompressGzip, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/states", ep), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req2CompressGzip.Header.Set(server.RequestHeaderContentType, server.RequestHeaderJSON)
	req2CompressGzip.Header.Set(server.RequestHeaderAcceptEncoding, server.RequestHeaderEncodingGzip)
	resp2CompressGzip, err := client.Do(req2CompressGzip)
	if err != nil {
		t.Errorf("failed to make request to /v1/states: %v", err)
	}
	defer resp2CompressGzip.Body.Close()
	if resp2CompressGzip.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %v", resp2CompressGzip.Status)
	}
	gr, err := gzip.NewReader(resp2CompressGzip.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	b2CompressGzip, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to read gzip: %v", err)
	}
	if err := json.Unmarshal(b2CompressGzip, &componentStates); err != nil {
		t.Errorf("failed to unmarshal states: %v", err)
	}
	found = false
	for _, comp := range componentStates {
		if comp.Component != "info" {
			continue
		}
		for _, state := range comp.States {
			if state.Name != "annotations" {
				continue
			}

			found = true
			if state.ExtraInfo[randKey] != randVal {
				t.Errorf("expected annotation %q=%q, got %q (%s)", randKey, randVal, state.ExtraInfo[randKey], string(b2NoCompress))
			}
		}
	}
	if !found {
		t.Errorf("expected to find annotation state, got %v (%s)", componentStates, string(b2NoCompress))
	}

	req2CompressGzip3, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/states", ep), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req2CompressGzip3.Header.Set("Content-Type", "application/json")
	req2CompressGzip3.Header.Set("Accept-Encoding", "gzip")
	resp2CompressGzip2, err := client.Do(req2CompressGzip3)
	if err != nil {
		t.Errorf("failed to make request to /v1/states: %v", err)
	}
	defer resp2CompressGzip2.Body.Close()
	if resp2CompressGzip2.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %v", resp2CompressGzip2.Status)
	}
	if b, err := io.ReadAll(resp2CompressGzip2.Body); err != nil {
		t.Errorf("failed to read response body: %v", err)
	} else {
		t.Logf("resp2CompressGzip2 size: %d", len(b))
	}

	req3NoCompress, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/metrics?since=10h", ep), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req3NoCompress.Header.Set("Content-Type", "application/json")
	resp3NoCompress, err := client.Do(req3NoCompress)
	if err != nil {
		t.Errorf("failed to make request to /v1/states: %v", err)
	}
	defer resp3NoCompress.Body.Close()
	if resp3NoCompress.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %v", resp3NoCompress.Status)
	}
	b3NoCompress, err := io.ReadAll(resp3NoCompress.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	t.Logf("resp3NoCompress size: %d", len(b3NoCompress))
	var metrics v1.LeptonMetrics
	if err := json.Unmarshal(b3NoCompress, &metrics); err != nil {
		t.Errorf("failed to unmarshal metrics: %v", err)
	}

	req3CompressGzip, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/metrics?since=10h", ep), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req3CompressGzip.Header.Set("Content-Type", "application/json")
	req3CompressGzip.Header.Set("Accept-Encoding", "gzip")
	resp3CompressGzip, err := client.Do(req3CompressGzip)
	if err != nil {
		t.Errorf("failed to make request to /v1/states: %v", err)
	}
	defer resp3CompressGzip.Body.Close()
	if resp3CompressGzip.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %v", resp3CompressGzip.Status)
	}
	gr, err = gzip.NewReader(resp3CompressGzip.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	b3CompressGzip, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to read gzip: %v", err)
	}
	if err := json.Unmarshal(b3CompressGzip, &metrics); err != nil {
		t.Errorf("failed to unmarshal metrics: %v", err)
	}

	req3CompressGzip2, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/metrics?since=10h", ep), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req3CompressGzip2.Header.Set("Content-Type", "application/json")
	req3CompressGzip2.Header.Set("Accept-Encoding", "gzip")
	resp3CompressGzip2, err := client.Do(req3CompressGzip2)
	if err != nil {
		t.Errorf("failed to make request to /v1/states: %v", err)
	}
	defer resp3CompressGzip2.Body.Close()
	if resp3CompressGzip2.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %v", resp3CompressGzip2.Status)
	}
	if b, err := io.ReadAll(resp3CompressGzip2.Body); err != nil {
		t.Errorf("failed to read response body: %v", err)
	} else {
		t.Logf("resp3CompressGzip2 size: %d", len(b))
	}

	reqMetrics, err := http.NewRequest("GET", fmt.Sprintf("https://%s/metrics", ep), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	respMetrics, err := client.Do(reqMetrics)
	if err != nil {
		t.Errorf("failed to make request to /v1/states: %v", err)
	}
	defer respMetrics.Body.Close()
	if respMetrics.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %v", respMetrics.Status)
	}
	metricsBytes, err := io.ReadAll(respMetrics.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	t.Logf("respMetrics size:\n%s", string(metricsBytes))

	t.Log("now testing with client/v1")
	components, err := client_v1.GetComponents(ctx, ep, client_v1.WithRequestContentTypeJSON(), client_v1.WithAcceptEncodingGzip())
	if err != nil {
		t.Errorf("failed to get components: %v", err)
	}
	t.Logf("components: %v", components)

	info, err := client_v1.GetInfo(ctx, ep, client_v1.WithRequestContentTypeJSON(), client_v1.WithAcceptEncodingGzip())
	if err != nil {
		t.Errorf("failed to get info: %v", err)
	}
	t.Logf("info: %v", info)

	states, err := client_v1.GetStates(ctx, ep, client_v1.WithRequestContentTypeYAML(), client_v1.WithAcceptEncodingGzip())
	if err != nil {
		t.Errorf("failed to get states: %v", err)
	}
	t.Logf("states: %v", states)

	events, err := client_v1.GetEvents(ctx, ep, client_v1.WithRequestContentTypeYAML(), client_v1.WithAcceptEncodingGzip())
	if err != nil {
		t.Errorf("failed to get events: %v", err)
	}
	t.Logf("events: %v", events)

	metricsV1, err := client_v1.GetMetrics(ctx, ep, client_v1.WithAcceptEncodingGzip())
	if err != nil {
		t.Errorf("failed to get metricsV1: %v", err)
	}
	t.Logf("metricsV1: %v", metricsV1)
}

func randStr(t *testing.T, length int) string {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		t.Fatal(err)
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length]
}
