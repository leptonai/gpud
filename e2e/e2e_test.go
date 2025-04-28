package e2e_test

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	clientv1 "github.com/leptonai/gpud/client/v1"
	mocklspci "github.com/leptonai/gpud/e2e/mock/lspci"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/errdefs"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
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
	gCtx, gCancel := context.WithTimeout(context.Background(), 10*time.Minute)

	randSfx, err := randStr(32)
	Expect(err).NotTo(HaveOccurred(), "failed to rand key")
	initPluginWriteFile := filepath.Join(os.TempDir(), randSfx)

	initPluginWriteFileContents, err := randStr(128)
	Expect(err).NotTo(HaveOccurred(), "failed to rand contents")

	BeforeAll(func() {
		err = os.Setenv(nvmllib.EnvMockAllSuccess, "true")
		Expect(err).NotTo(HaveOccurred(), "failed to set "+nvmllib.EnvMockAllSuccess)

		By("mock lspci")
		err = mocklspci.Mock(mocklspci.NormalOutput)
		Expect(err).ToNot(HaveOccurred(), "failed to mock lspci")

		By("start gpud scan")
		cmd = exec.CommandContext(gCtx, os.Getenv("GPUD_BIN"), "scan")
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

		By("create init plugin specs")
		initPluginSpecs := pkgcustomplugins.Specs{
			{
				PluginName: "init-plugin",
				Type:       pkgcustomplugins.SpecTypeInit,
				HealthStatePlugin: &pkgcustomplugins.Plugin{
					Steps: []pkgcustomplugins.Step{
						{
							Name: "first-step",
							RunBashScript: &pkgcustomplugins.RunBashScript{
								ContentType: "plaintext",
								Script:      "echo hello",
							},
						},
						{
							Name: "second-step",
							RunBashScript: &pkgcustomplugins.RunBashScript{
								ContentType: "plaintext",
								Script:      fmt.Sprintf("echo %s > %s", initPluginWriteFileContents, initPluginWriteFile),
							},
						},
					},
				},
				Timeout: metav1.Duration{Duration: time.Minute},
			},
		}
		specsB, err := yaml.Marshal(initPluginSpecs)
		Expect(err).NotTo(HaveOccurred(), "failed to marshal init plugin specs")

		specFile, err := os.CreateTemp(os.TempDir(), "plugins.yaml")
		Expect(err).NotTo(HaveOccurred(), "failed to create temp file")
		defer os.Remove(specFile.Name())
		_, err = specFile.Write(specsB)
		Expect(err).NotTo(HaveOccurred(), "failed to write init plugin specs")
		Expect(specFile.Close()).To(Succeed(), "failed to close temp file")

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

			// to run e2e test with api plugin registration
			"--enable-plugin-api",
			"--plugin-specs-file=" + specFile.Name(),
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

	var rootCtx context.Context
	var rootCancel context.CancelFunc
	BeforeEach(func() {
		rootCtx, rootCancel = context.WithTimeout(context.Background(), 3*time.Minute)
	})
	AfterEach(func() {
		rootCancel()
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
					if len(state.ExtraInfo) == 0 {
						continue
					}
					if !strings.Contains(state.ExtraInfo["data"], "annotations") {
						continue
					}

					found = true
					Expect(state.ExtraInfo["data"]).To(ContainSubstring(randVal), fmt.Sprintf("unexpected annotations from %q", string(body)))
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
					if len(state.ExtraInfo) == 0 {
						continue
					}
					if !strings.Contains(state.ExtraInfo["data"], "annotations") {
						continue
					}

					found = true
					Expect(state.ExtraInfo["data"]).To(ContainSubstring(randVal), fmt.Sprintf("unexpected annotations from %q", string(body)))
				}
			}
			Expect(found).To(BeTrue(), fmt.Sprintf("expected to find annotation state, got %v (%s)", componentStates, string(body)))
		})
	})

	Describe("/v1/metrics requests", func() {
		It("request without compress", func() {
			// enough time for metrics to be collected
			time.Sleep(time.Minute + 30*time.Second)

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
			Expect(found["disk"]).To(BeTrue(), "expected disk component to be present")
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
			states, err := clientv1.GetHealthStates(rootCtx, "https://"+ep, clientv1.WithComponent("disk"))
			Expect(err).NotTo(HaveOccurred(), "failed to get disk states")
			for _, ss := range states {
				for _, s := range ss.States {
					GinkgoLogr.Info(fmt.Sprintf("state: %q, health: %s, extra info: %q\n", s.Name, s.Health, s.ExtraInfo))
				}
			}
		})

		for _, opts := range [][]clientv1.OpOption{
			{clientv1.WithRequestContentTypeJSON()},
			{clientv1.WithRequestContentTypeYAML()},
			{clientv1.WithRequestContentTypeJSON(), clientv1.WithAcceptEncodingGzip()},
			{clientv1.WithRequestContentTypeYAML(), clientv1.WithAcceptEncodingGzip()},
		} {
			It("get states with options", func() {
				components, err := clientv1.GetHealthStates(rootCtx, "https://"+ep, opts...)
				Expect(err).NotTo(HaveOccurred(), "failed to get states")
				GinkgoLogr.Info("got components", "components", components)

				info, err := clientv1.GetInfo(rootCtx, "https://"+ep, opts...)
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

				_, err = clientv1.GetHealthStates(rootCtx, "https://"+ep, append(opts, clientv1.WithComponent("unknown!!!"))...)
				Expect(err).To(Equal(errdefs.ErrNotFound), "expected ErrNotFound")
			})
		}
	})

	Describe("register custom plugin with client/v1", func() {

		It("make sure init plugin has run", func() {
			b, err := os.ReadFile(initPluginWriteFile)
			Expect(err).NotTo(HaveOccurred(), "failed to read init plugin file")
			Expect(string(b)).To(ContainSubstring(initPluginWriteFileContents), "expected init plugin to have run")
		})

		It("list custom plugins", func() {
			csPlugins, err := clientv1.GetCustomPlugins(rootCtx, "https://"+ep)
			Expect(err).NotTo(HaveOccurred(), "failed to get custom plugins")
			GinkgoLogr.Info("got custom plugins", "custom plugins", csPlugins)
			Expect(csPlugins).To(BeEmpty(), "expected no custom plugins")
		})

		pluginName, err := randStr(10)
		Expect(err).NotTo(HaveOccurred(), "failed to rand str")

		componentName := pkgcustomplugins.ConvertToComponentName(pluginName)

		// register with manual mode first
		randSfx1, err := randStr(10)
		Expect(err).NotTo(HaveOccurred(), "failed to rand suffix")
		fileToWrite1 := filepath.Join(os.TempDir(), "testplugin"+randSfx1)
		defer os.Remove(fileToWrite1)

		It("register a custom plugin with manual mode", func() {
			testPluginSpec := pkgcustomplugins.Spec{
				PluginName: pluginName,
				Type:       pkgcustomplugins.SpecTypeComponent,

				// should not run, only registers
				Mode: "manual",

				HealthStatePlugin: &pkgcustomplugins.Plugin{
					Steps: []pkgcustomplugins.Step{
						{
							Name: "first-step",
							RunBashScript: &pkgcustomplugins.RunBashScript{
								Script:      "echo 'hello'",
								ContentType: "plaintext",
							},
						},
						{
							Name: "second-step",
							RunBashScript: &pkgcustomplugins.RunBashScript{
								Script:      "echo 'world'",
								ContentType: "plaintext",
							},
						},
						{
							Name: "third-step",
							RunBashScript: &pkgcustomplugins.RunBashScript{
								Script:      "echo 111 > " + fileToWrite1,
								ContentType: "plaintext",
							},
						},
					},
				},
				Timeout:  metav1.Duration{Duration: 30 * time.Second},
				Interval: metav1.Duration{Duration: 0},
			}

			rerr := clientv1.RegisterCustomPlugin(rootCtx, "https://"+ep, testPluginSpec)
			Expect(rerr).NotTo(HaveOccurred(), "failed to register custom plugin")

			// redundant registration request should fail
			rerr = clientv1.RegisterCustomPlugin(rootCtx, "https://"+ep, testPluginSpec)
			Expect(rerr).To(HaveOccurred(), "expected to fail with redundant registration")
		})

		It("list custom plugins and make sure the plugin is registered even with manual mode", func() {
			csPlugins, err := clientv1.GetCustomPlugins(rootCtx, "https://"+ep)
			Expect(err).NotTo(HaveOccurred(), "failed to get custom plugins")
			GinkgoLogr.Info("got custom plugins", "custom plugins", csPlugins)
			for componentName, curSpec := range csPlugins {
				Expect(componentName).Should(Equal(curSpec.ComponentName()))
				GinkgoLogr.Info("currently registered custom plugin (expect mode: manual)", "name", curSpec.PluginName, "componentName", componentName)

				b, err := json.Marshal(curSpec)
				Expect(err).NotTo(HaveOccurred(), "failed to marshal spec")
				fmt.Println("currently registered custom plugin (expect mode: manual)", "name", curSpec.PluginName, "componentName", componentName, "spec", string(b))

				Expect(curSpec.Mode).Should(Equal("manual"), "expected manual mode")
			}
			Expect(csPlugins[componentName]).NotTo(BeNil(), "expected to be registered")
		})

		It("make sure the plugin has been not run as it's manual mode", func() {
			// wait for the plugin to run
			time.Sleep(3 * time.Second)

			_, err := os.Stat(fileToWrite1)
			Expect(errors.Is(err, os.ErrNotExist)).Should(BeTrue(), "expected file to not be created")
		})

		It("trigger the plugin that is in manual mode", func() {
			resp, err := clientv1.TriggerComponentCheck(rootCtx, "https://"+ep, componentName)
			Expect(err).NotTo(HaveOccurred(), "failed to get custom plugins")
			Expect(len(resp)).To(Equal(1), "expected 1 response")

			fmt.Printf("%+v\n", resp)
		})

		It("make sure the plugin has been run manually", func() {
			_, err := os.Stat(fileToWrite1)
			Expect(err).NotTo(HaveOccurred(), "expected file to be created")
		})

		randSfx2, err := randStr(10)
		Expect(err).NotTo(HaveOccurred(), "failed to rand suffix")
		fileToWrite2 := filepath.Join(os.TempDir(), "testplugin"+randSfx2)
		defer os.Remove(fileToWrite2)

		randStrToEcho, err := randStr(100)
		Expect(err).NotTo(HaveOccurred(), "failed to rand suffix")

		It("updates the custom plugin with non-manual mode", func() {
			testPluginSpec := pkgcustomplugins.Spec{
				PluginName: pluginName,
				Type:       pkgcustomplugins.SpecTypeComponent,

				// should not run, only registers
				Mode: "manual",

				HealthStatePlugin: &pkgcustomplugins.Plugin{
					Steps: []pkgcustomplugins.Step{
						{
							Name: "first-step",
							RunBashScript: &pkgcustomplugins.RunBashScript{
								Script:      "echo 'hello'",
								ContentType: "plaintext",
							},
						},
						{
							Name: "second-step",
							RunBashScript: &pkgcustomplugins.RunBashScript{
								Script:      "echo 'world'",
								ContentType: "plaintext",
							},
						},
						{
							Name: "third-step",
							RunBashScript: &pkgcustomplugins.RunBashScript{
								Script:      "echo 111 > " + fileToWrite1,
								ContentType: "plaintext",
							},
						},
					},
				},
				Timeout:  metav1.Duration{Duration: 30 * time.Second},
				Interval: metav1.Duration{Duration: 0},
			}
			testPluginSpec.Interval = metav1.Duration{Duration: time.Minute}
			testPluginSpec.Mode = ""
			testPluginSpec.HealthStatePlugin.Steps = append(testPluginSpec.HealthStatePlugin.Steps,
				pkgcustomplugins.Step{
					Name: "fourth-step",
					RunBashScript: &pkgcustomplugins.RunBashScript{
						Script:      "echo 111 > " + fileToWrite2,
						ContentType: "plaintext",
					},
				},
			)
			testPluginSpec.HealthStatePlugin.Steps = append(testPluginSpec.HealthStatePlugin.Steps, pkgcustomplugins.Step{
				Name: "fifth-step",
				RunBashScript: &pkgcustomplugins.RunBashScript{
					Script:      `echo '{"name":"` + randStrToEcho + `", "health":"degraded"}'`,
					ContentType: "plaintext",
				},
			})
			testPluginSpec.HealthStatePlugin.Parser = &pkgcustomplugins.PluginOutputParseConfig{
				JSONPaths: []pkgcustomplugins.JSONPath{
					{Field: "name", Query: "$.name"},
					{Field: "health", Query: "$.health"},

					// non-existent path should be skipped
					{Field: "nonexistent1", Query: "$.nonexistent"},
					{Field: "nonexistent2", Query: "$.a.b.c.d.e"},
				},
			}

			rerr := clientv1.UpdateCustomPlugin(rootCtx, "https://"+ep, testPluginSpec)
			Expect(rerr).NotTo(HaveOccurred(), "failed to update custom plugin")

			// list custom plugins and make sure the plugin is registered with non-manual mode
			csPlugins, err := clientv1.GetCustomPlugins(rootCtx, "https://"+ep)
			Expect(err).NotTo(HaveOccurred(), "failed to get custom plugins")
			GinkgoLogr.Info("got custom plugins", "custom plugins", csPlugins)
			for componentName, curSpec := range csPlugins {
				Expect(componentName).Should(Equal(curSpec.ComponentName()))
				GinkgoLogr.Info("currently registered custom plugin (expect mode: '')", "name", curSpec.PluginName, "componentName", componentName)

				b, err := json.Marshal(curSpec)
				Expect(err).NotTo(HaveOccurred(), "failed to marshal spec")
				fmt.Println("currently registered custom plugin (expect mode: '')", "name", curSpec.PluginName, "componentName", componentName, "spec", string(b))

				Expect(curSpec.Mode).Should(BeEmpty(), "expected empty mode")
			}
			Expect(csPlugins[componentName]).NotTo(BeNil(), "expected to be registered")

			// make sure the plugin has been run once by checking the file exists when the dry mode is disabled
			// wait for the plugin to run
			time.Sleep(3 * time.Second)

			_, err = os.Stat(fileToWrite2)
			Expect(err).NotTo(HaveOccurred(), "expected file to be created")

			req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/states", ep), nil)
			Expect(err).NotTo(HaveOccurred(), "failed to create request")
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred(), "failed to make request")
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred(), "failed to read response body")
			fmt.Println("/v1/states RESPONSE BODY:", string(body))

			states, err := clientv1.GetHealthStates(rootCtx, "https://"+ep, clientv1.WithComponent(componentName))
			Expect(err).NotTo(HaveOccurred(), "failed to get states")
			GinkgoLogr.Info("got states", "states", states)
			Expect(states).ToNot(BeEmpty(), "expected states to not be empty")
			Expect(states[0].States).To(HaveLen(1), "expected states to have 1 state")
			Expect(states[0].States[0].Health).To(Equal(apiv1.HealthStateTypeHealthy), "expected health state to be healthy")
			Expect(states[0].States[0].Reason).To(Equal("ok"), "expected reason to be ok")
			Expect(states[0].States[0].ExtraInfo["name"]).To(Equal(randStrToEcho))
			Expect(states[0].States[0].ExtraInfo["health"]).To(Equal("degraded"))
			Expect(states[0].States[0].ExtraInfo["last_check_ts_unix_seconds"]).Should(Not(BeEmpty()))
			Expect(states[0].States[0].ExtraInfo["last_check_ts_unix_seconds"]).Should(BeNumerically("<", time.Now().Unix()+10))
		})

		It("deregister the custom plugin", func() {
			derr := clientv1.DeregisterComponent(rootCtx, "https://"+ep, componentName)
			Expect(derr).NotTo(HaveOccurred(), "failed to deregister custom plugin")
		})

		It("list custom plugins and make sure the plugin has been de-registered", func() {
			csPlugins, err := clientv1.GetCustomPlugins(rootCtx, "https://"+ep)
			Expect(err).NotTo(HaveOccurred(), "failed to get custom plugins")
			GinkgoLogr.Info("got custom plugins", "custom plugins", csPlugins)
			Expect(csPlugins).To(BeEmpty(), "expected no custom plugins")
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
