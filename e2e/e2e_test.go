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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "github.com/leptonai/gpud/api/v1"
	client_v1 "github.com/leptonai/gpud/client/v1"
	nvidia_hw_slowdown_id "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown/id"
	mocklspci "github.com/leptonai/gpud/e2e/mock/lspci"
	mocknvidiasmi "github.com/leptonai/gpud/e2e/mock/nvidia-smi"
	mocknvml "github.com/leptonai/gpud/e2e/mock/nvml"
	"github.com/leptonai/gpud/pkg/errdefs"
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
		err = os.Setenv(mocknvml.EnvNVMLMock, "true")
		Expect(err).NotTo(HaveOccurred(), "failed to set GPUD_MOCK_NVML")

		By("mock lspci")
		err = mocklspci.Mock(mocklspci.NormalOutput)
		Expect(err).ToNot(HaveOccurred(), "failed to mock lspci")

		By("mock nvidia-smi")
		err = mocknvidiasmi.Mock(mocknvidiasmi.NormalSMIOutput, mocknvidiasmi.NormalSMIOutput)
		Expect(err).ToNot(HaveOccurred(), "failed to mock nvidia-smi")

		By("start gpud scan")
		cmd = exec.CommandContext(gCtx, os.Getenv("GPUD_BIN"), "scan", "--dmesg-check=false")
		b, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to run gpud scan:\n%s", string(b)))
		GinkgoLogr.Info("gpud scan successfully", "output", string(b))

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
		cmd = exec.CommandContext(gCtx, os.Getenv("GPUD_BIN"), "run",
			"--log-level=debug",
			"--web-enable=false",
			"--enable-auto-update=false",
			"--annotations", fmt.Sprintf("{%q:%q}", randKey, randVal),
			fmt.Sprintf("--listen-address=%s", ep),
		)
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
			GinkgoLogr.Info("response size", "size", string(body))

			var componentStates []v1.LeptonComponentStates
			err = json.Unmarshal(body, &componentStates)
			Expect(err).NotTo(HaveOccurred(), "failed to unmarshal response body")

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
					Expect(state.ExtraInfo[randKey]).To(Equal(randVal), fmt.Sprintf("unexpected annotations from %q", string(body)))
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

			var componentStates []v1.LeptonComponentStates
			err = json.Unmarshal(body, &componentStates)
			Expect(err).NotTo(HaveOccurred(), "failed to unmarshal response body")

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
					Expect(state.ExtraInfo[randKey]).To(Equal(randVal), fmt.Sprintf("unexpected annotations from %q", string(body)))
				}
			}
			Expect(found).To(BeTrue(), fmt.Sprintf("expected to find annotation state, got %v (%s)", componentStates, string(body)))
		})

	})

	Describe("/v1/metrics requests", func() {

		It("request without compress", func() {
			req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/metrics", ep), nil)
			Expect(err).NotTo(HaveOccurred(), "failed to create request")

			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred(), "failed to make request")
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred(), "failed to read response body")
			GinkgoLogr.Info("response size", "size", string(body))

			var metrics v1.LeptonMetrics
			err = json.Unmarshal(body, &metrics)
			Expect(err).NotTo(HaveOccurred(), "failed to unmarshal response body")
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

			var metrics v1.LeptonMetrics
			err = json.Unmarshal(body, &metrics)
			Expect(err).NotTo(HaveOccurred(), "failed to unmarshal response body")
		})

	})

	Describe("states with client/v1", func() {

		It("get disk states", func() {
			states, err := client_v1.GetStates(ctx, "https://"+ep, client_v1.WithComponent("disk"))
			Expect(err).NotTo(HaveOccurred(), "failed to get disk states")
			for _, ss := range states {
				for _, s := range ss.States {
					GinkgoLogr.Info(fmt.Sprintf("state: %q, healthy: %v, extra info: %q\n", s.Name, s.Healthy, s.ExtraInfo))
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
				components, err := client_v1.GetStates(ctx, "https://"+ep, opts...)
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
						GinkgoLogr.Info("metric", "name", metric.MetricName, "value", metric.Value)
					}
					for _, state := range i.Info.States {
						GinkgoLogr.Info("state", "name", state.Name, "healthy", state.Healthy)
					}
				}

				_, err = client_v1.GetStates(ctx, "https://"+ep, append(opts, client_v1.WithComponent("unknown!!!"))...)
				Expect(err).To(Equal(errdefs.ErrNotFound), "expected ErrNotFound")
			})
		}
	})

	Describe("HW Slowdown test", func() {

		// TODO: fix the hw slowdown cases for events interval

		It("should report healthy for normal case", func() {
			states, err := client_v1.GetStates(ctx, "https://"+ep, client_v1.WithComponent(nvidia_hw_slowdown_id.Name))
			Expect(err).NotTo(HaveOccurred(), "failed to get hw slowdown states")

			GinkgoLogr.Info("got states", "states", states)
		})

		It("should report unhealthy for slowdown case", func() {
			err = mocknvidiasmi.Mock(mocknvidiasmi.NormalSMIOutput, mocknvidiasmi.HWSlowdownQueryOutput)
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				err = mocknvidiasmi.Mock(mocknvidiasmi.NormalSMIOutput, mocknvidiasmi.NormalQueryOutput)
				Expect(err).NotTo(HaveOccurred())
			}()

			states, err := client_v1.GetStates(ctx, "https://"+ep, client_v1.WithComponent(nvidia_hw_slowdown_id.Name))
			Expect(err).NotTo(HaveOccurred(), "failed to get hw slowdown states")

			GinkgoLogr.Info("got states", "states", states)
		})
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
