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
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiv1 "github.com/leptonai/gpud/api/v1"
	client_v1 "github.com/leptonai/gpud/client/v1"
	mocklspci "github.com/leptonai/gpud/e2e/mock/lspci"
	"github.com/leptonai/gpud/pkg/errdefs"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/server"
)

func TestE2E(t *testing.T) {
	if os.Getenv("GPUD_BIN") == "" {
		t.Skip("skipping e2e tests")
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite")
}

var (
	cmd              *exec.Cmd
	ep               string
	randKey, randVal string

	client = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
)

var _ = Describe("[GPUD E2E]", Ordered, func() {
	var err error
	gCtx, gCancel := context.WithTimeout(context.Background(), time.Minute*8)

	BeforeAll(func() {
		err = os.Setenv(nvml_lib.EnvMockAllSuccess, "true")
		Expect(err).NotTo(HaveOccurred(), "failed to set "+nvml_lib.EnvMockAllSuccess)

		By("mock lspci")
		err = mocklspci.Mock(mocklspci.NormalOutput)
		Expect(err).ToNot(HaveOccurred(), "failed to mock lspci")

		By("start gpud scan")
		cmd = exec.CommandContext(gCtx, os.Getenv("GPUD_BIN"), "scan", "--kmsg-check=false")
		b, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to run gpud scan:\n%s", string(b)))
		GinkgoLogr.Info("gpud scan successfully", "output", string(b))
		fmt.Println("'gpud scan' OUTPUT:", string(b))

		By("get an available port")
		listener, err := net.Listen("tcp", "localhost:0")
		Expect(err).NotTo(HaveOccurred(), "failed to find a free port")
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		ep = fmt.Sprintf("localhost:%d", port)

		randKey, err = randStr(10)
		Expect(err).NotTo(HaveOccurred(), "failed to rand key")
		randVal, err = randStr(10)
		Expect(err).NotTo(HaveOccurred(), "failed to rand value")

		By("start gpud command")

		args := []string{
			"run",
			`--log-file=""`, // stdout/stderr
			"--log-level=debug",
			"--enable-auto-update=false",
			"--annotations", fmt.Sprintf("{%q:%q}", randKey, randVal),
			fmt.Sprintf("--listen-address=%s", ep),
		}

		cmd = exec.CommandContext(gCtx, os.Getenv("GPUD_BIN"), args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Start()
		Expect(err).NotTo(HaveOccurred(), "failed to start gpud")

		By("waiting for gpud started")
		Eventually(func() error {
			req1NoCompress, err := http.NewRequest("GET", fmt.Sprintf("https://%s/healthz", ep), nil)
			Expect(err).NotTo(HaveOccurred(), "failed to create request")

			resp1, err := client.Do(req1NoCompress)
			if err != nil {
				return fmt.Errorf("request failed: %w", err)
			}

			defer resp1.Body.Close()
			if resp1.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected healthz status: %s", resp1.Status)
			}

			b1, err := io.ReadAll(resp1.Body)
			Expect(err).NotTo(HaveOccurred(), "failed to read response body")

			expectedBody := `{"status":"ok","version":"v1"}`
			if string(b1) != expectedBody {
				return fmt.Errorf("unexpected response body: %s", string(b1))
			}
			GinkgoLogr.Info("success health check", "response", string(b1), "ep", ep)
			fmt.Println("/healthz RESPONSE:", string(b1))

			return nil
		}).WithTimeout(15*time.Second).WithPolling(3*time.Second).ShouldNot(HaveOccurred(), "failed to wait for gpud started")
		By("gpud started")
	})

	AfterAll(func() {
		By("stop gpud command")
		gCancel()
		err := cmd.Process.Kill()
		Expect(err).NotTo(HaveOccurred(), "failed to kill gpud process")
	})

	var ctx context.Context
	var cancel context.CancelFunc
	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), time.Minute)
	})
	AfterEach(func() {
		cancel()
	})

	Describe("/v1/states requests", func() {

		It("request without compress", func() {
			req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/states", ep), nil)
			Expect(err).NotTo(HaveOccurred(), "failed to create request")

			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred(), "failed to make request")
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred(), "failed to read response body")
			fmt.Println("/v1/states RESPONSE SIZE:", len(body))
			GinkgoLogr.Info("/v1/states response size", "size", string(body))
			fmt.Println("/v1/states RESPONSE BODY:", string(body))
			GinkgoLogr.Info("/v1/states response", "response", string(body))

			var componentStates []apiv1.ComponentHealthStates
			err = json.Unmarshal(body, &componentStates)
			Expect(err).NotTo(HaveOccurred(), "failed to unmarshal response body")

			found := false
			for _, comp := range componentStates {
				if comp.Component != "info" {
					continue
				}
				for _, state := range comp.States {
					if len(state.DeprecatedExtraInfo) == 0 {
						continue
					}
					if !strings.Contains(state.DeprecatedExtraInfo["data"], "annotations") {
						continue
					}

					found = true
					Expect(state.DeprecatedExtraInfo["data"]).To(ContainSubstring(randVal), fmt.Sprintf("unexpected annotations from %q", string(body)))
				}
			}
			Expect(found).To(BeTrue(), fmt.Sprintf("expected to find annotation state, got %v (%s)", componentStates, string(body)))
		})

		It("request with compress", func() {
			req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/states", ep), nil)
			Expect(err).NotTo(HaveOccurred(), "failed to create request")

			req.Header.Set(server.RequestHeaderContentType, server.RequestHeaderJSON)
			req.Header.Set(server.RequestHeaderAcceptEncoding, server.RequestHeaderEncodingGzip)

			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred(), "failed to make request")
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			gr, err := gzip.NewReader(resp.Body)
			Expect(err).NotTo(HaveOccurred(), "failed to create gzip reader")
			body, err := io.ReadAll(gr)
			Expect(err).NotTo(HaveOccurred(), "failed to read gzip")

			var componentStates []apiv1.ComponentHealthStates
			err = json.Unmarshal(body, &componentStates)
			Expect(err).NotTo(HaveOccurred(), "failed to unmarshal response body")

			found := false
			for _, comp := range componentStates {
				if comp.Component != "info" {
					continue
				}
				for _, state := range comp.States {
					if len(state.DeprecatedExtraInfo) == 0 {
						continue
					}
					if !strings.Contains(state.DeprecatedExtraInfo["data"], "annotations") {
						continue
					}

					found = true
					Expect(state.DeprecatedExtraInfo["data"]).To(ContainSubstring(randVal), fmt.Sprintf("unexpected annotations from %q", string(body)))
				}
			}
			Expect(found).To(BeTrue(), fmt.Sprintf("expected to find annotation state, got %v (%s)", componentStates, string(body)))
		})
	})

	Describe("/v1/metrics requests", func() {
		It("request without compress", func() {
			// enough time for metrics to be collected
			time.Sleep(2 * time.Minute)

			req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/metrics", ep), nil)
			Expect(err).NotTo(HaveOccurred(), "failed to create request")

			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred(), "failed to make request")
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred(), "failed to read response body")
			fmt.Println("/v1/metrics RESPONSE SIZE:", len(body))
			GinkgoLogr.Info("/v1/metrics response size", "size", string(body))
			fmt.Println("/v1/metrics RESPONSE BODY:", string(body))
			GinkgoLogr.Info("/v1/metrics response", "response", string(body))

			var metrics apiv1.GPUdComponentMetrics
			err = json.Unmarshal(body, &metrics)
			Expect(err).NotTo(HaveOccurred(), "failed to unmarshal response body")

			// should not be empty
			Expect(metrics).ToNot(BeEmpty(), "expected metrics to not be empty")

			// make sure default components are present (enabled by default)
			found := make(map[string]bool)
			for _, m := range metrics {
				found[m.Component] = true
			}
			Expect(found["cpu"]).To(BeTrue(), "expected cpu component to be present")
			Expect(found["memory"]).To(BeTrue(), "expected memory component to be present")
			Expect(found["network-latency"]).To(BeTrue(), "expected network-latency component to be present")
		})

		It("request with compress", func() {
			req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/metrics", ep), nil)
			Expect(err).NotTo(HaveOccurred(), "failed to create request")

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept-Encoding", "gzip")

			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred(), "failed to make request")
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			gr, err := gzip.NewReader(resp.Body)
			Expect(err).NotTo(HaveOccurred(), "failed to create gzip reader")
			body, err := io.ReadAll(gr)
			Expect(err).NotTo(HaveOccurred(), "failed to read response body")

			var metrics apiv1.GPUdComponentMetrics
			err = json.Unmarshal(body, &metrics)
			Expect(err).NotTo(HaveOccurred(), "failed to unmarshal response body")
		})
	})

	Describe("/metrics requests", func() {

		It("request prometheus metrics without compress", func() {
			req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/metrics", ep), nil)
			Expect(err).NotTo(HaveOccurred(), "failed to create request")

			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred(), "failed to make request")
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred(), "failed to read response body")

			fmt.Println("/metrics RESPONSE SIZE:", len(body))
			GinkgoLogr.Info("/metrics response size", "size", string(body))
			fmt.Println("/metrics RESPONSE BODY:", string(body))
			GinkgoLogr.Info("/metrics response", "response", string(body))
		})

	})

	Describe("states with client/v1", func() {

		It("get disk states", func() {
			states, err := client_v1.GetHealthStates(ctx, "https://"+ep, client_v1.WithComponent("disk"))
			Expect(err).NotTo(HaveOccurred(), "failed to get disk states")
			for _, ss := range states {
				for _, s := range ss.States {
					GinkgoLogr.Info(fmt.Sprintf("state: %q, health: %s, extra info: %q\n", s.Name, s.Health, s.DeprecatedExtraInfo))
				}
			}
		})

		for _, opts := range [][]client_v1.OpOption{
			{client_v1.WithRequestContentTypeJSON()},
			{client_v1.WithRequestContentTypeYAML()},
			{client_v1.WithRequestContentTypeJSON(), client_v1.WithAcceptEncodingGzip()},
			{client_v1.WithRequestContentTypeYAML(), client_v1.WithAcceptEncodingGzip()},
		} {
			It("get states with options", func() {
				components, err := client_v1.GetHealthStates(ctx, "https://"+ep, opts...)
				Expect(err).NotTo(HaveOccurred(), "failed to get states")
				GinkgoLogr.Info("got components", "components", components)

				info, err := client_v1.GetInfo(ctx, "https://"+ep, opts...)
				Expect(err).NotTo(HaveOccurred(), "failed to get info")
				GinkgoLogr.Info("got info", "info", info)

				By("getting component information")
				for _, i := range info {
					GinkgoLogr.Info("component", "name", i.Component)
					for _, event := range i.Info.Events {
						GinkgoLogr.Info("event", "name", event.Name, "message", event.Message)
					}
					for _, metric := range i.Info.Metrics {
						GinkgoLogr.Info("metric", "name", metric.DeprecatedMetricName, "value", metric.Value)
					}
					for _, state := range i.Info.States {
						GinkgoLogr.Info("state", "name", state.Name, "health", state.Health)
					}
				}

				_, err = client_v1.GetHealthStates(ctx, "https://"+ep, append(opts, client_v1.WithComponent("unknown!!!"))...)
				Expect(err).To(Equal(errdefs.ErrNotFound), "expected ErrNotFound")
			})
		}
	})
})

func randStr(length int) (string, error) {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}
