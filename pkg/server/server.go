package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/pprof"
	stdos "os"
	"path"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidia_badenvs "github.com/leptonai/gpud/components/accelerator/nvidia/bad-envs"
	nvidia_clock_speed "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed"
	nvidia_ecc "github.com/leptonai/gpud/components/accelerator/nvidia/ecc"
	nvidia_fabric_manager "github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager"
	nvidia_gpm "github.com/leptonai/gpud/components/accelerator/nvidia/gpm"
	nvidia_gsp_firmware_mode "github.com/leptonai/gpud/components/accelerator/nvidia/gsp-firmware-mode"
	nvidia_hw_slowdown "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown"
	nvidia_infiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	nvidia_info "github.com/leptonai/gpud/components/accelerator/nvidia/info"
	nvidia_memory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	nvidia_nccl "github.com/leptonai/gpud/components/accelerator/nvidia/nccl"
	nvidia_nvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	nvidia_peermem "github.com/leptonai/gpud/components/accelerator/nvidia/peermem"
	nvidia_persistence_mode "github.com/leptonai/gpud/components/accelerator/nvidia/persistence-mode"
	nvidia_power "github.com/leptonai/gpud/components/accelerator/nvidia/power"
	nvidia_processes "github.com/leptonai/gpud/components/accelerator/nvidia/processes"
	nvidia_remapped_rows "github.com/leptonai/gpud/components/accelerator/nvidia/remapped-rows"
	nvidia_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/sxid"
	nvidia_temperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	nvidia_utilization "github.com/leptonai/gpud/components/accelerator/nvidia/utilization"
	nvidia_xid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	containerd_pod "github.com/leptonai/gpud/components/containerd/pod"
	"github.com/leptonai/gpud/components/cpu"
	"github.com/leptonai/gpud/components/disk"
	docker_container "github.com/leptonai/gpud/components/docker/container"
	"github.com/leptonai/gpud/components/fd"
	"github.com/leptonai/gpud/components/fuse"
	"github.com/leptonai/gpud/components/info"
	kernel_module "github.com/leptonai/gpud/components/kernel-module"
	kubelet_pod "github.com/leptonai/gpud/components/kubelet/pod"
	"github.com/leptonai/gpud/components/library"
	"github.com/leptonai/gpud/components/memory"
	network_latency "github.com/leptonai/gpud/components/network/latency"
	"github.com/leptonai/gpud/components/os"
	"github.com/leptonai/gpud/components/pci"
	"github.com/leptonai/gpud/components/tailscale"
	_ "github.com/leptonai/gpud/docs/apis"
	lepconfig "github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/eventstore"
	gpud_manager "github.com/leptonai/gpud/pkg/gpud-manager"
	metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	gpud_state "github.com/leptonai/gpud/pkg/gpud-state"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/login"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	pkgmetricsscraper "github.com/leptonai/gpud/pkg/metrics/scraper"
	pkgmetricsstore "github.com/leptonai/gpud/pkg/metrics/store"
	pkgmetricssyncer "github.com/leptonai/gpud/pkg/metrics/syncer"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidia_query_nvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/session"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// Server is the gpud main daemon
type Server struct {
	dbRW *sql.DB
	dbRO *sql.DB

	uid                string
	fifoPath           string
	fifo               *stdos.File
	session            *session.Session
	enableAutoUpdate   bool
	autoUpdateExitCode int
}

func New(ctx context.Context, config *lepconfig.Config, endpoint string, cliUID string, packageManager *gpud_manager.Manager) (_ *Server, retErr error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	stateFile := ":memory:"
	if config.State != "" {
		stateFile = config.State
	}
	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open state file (for read-write): %w", err)
	}
	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return nil, fmt.Errorf("failed to open state file (for read-only): %w", err)
	}

	eventStore, err := eventstore.New(dbRW, dbRO, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open events database: %w", err)
	}
	rebootEventStore := pkghost.NewRebootEventStore(eventStore)

	promScraper, err := pkgmetricsscraper.NewPrometheusScraper(pkgmetrics.DefaultGatherer())
	if err != nil {
		return nil, fmt.Errorf("failed to create scraper: %w", err)
	}
	metricsSQLiteStore, err := pkgmetricsstore.NewSQLiteStore(ctx, dbRW, dbRO, pkgmetricsstore.DefaultTableName)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics store: %w", err)
	}
	syncer := pkgmetricssyncer.NewSyncer(ctx, promScraper, metricsSQLiteStore, time.Minute, time.Minute, 3*24*time.Hour)
	syncer.Start()

	fifoPath, err := lepconfig.DefaultFifoFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get fifo path: %w", err)
	}
	s := &Server{
		dbRW: dbRW,
		dbRO: dbRO,

		fifoPath:           fifoPath,
		enableAutoUpdate:   config.EnableAutoUpdate,
		autoUpdateExitCode: config.AutoUpdateExitCode,
	}
	defer func() {
		if retErr != nil {
			s.Stop()
		}
	}()

	nvidiaInstalled, err := nvidia_query.GPUsInstalled(ctx)
	if err != nil {
		return nil, err
	}

	var nvmlInstanceV2 nvidia_query_nvml.InstanceV2
	if runtime.GOOS == "linux" && nvidiaInstalled {
		nvmlInstanceV2, err = nvidia_query_nvml.NewInstanceV2()
		if err != nil {
			return nil, fmt.Errorf("failed to create NVML instance: %w", err)
		}
	}

	if err := gpud_state.CreateTableMachineMetadata(ctx, dbRW); err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}
	if err := gpud_state.CreateTableAPIVersion(ctx, dbRW); err != nil {
		return nil, fmt.Errorf("failed to create api version table: %w", err)
	}
	ver, err := gpud_state.UpdateAPIVersionIfNotExists(ctx, dbRW, "v1")
	if err != nil {
		return nil, fmt.Errorf("failed to update api version: %w", err)
	}
	log.Logger.Infow("api version", "version", ver)
	if ver != "v1" {
		return nil, fmt.Errorf("api version mismatch: %s (only supports v1)", ver)
	}

	if err := components_metrics_state.CreateTableMetrics(ctx, dbRW, components_metrics_state.DefaultTableName); err != nil {
		return nil, fmt.Errorf("failed to create metrics table: %w", err)
	}
	go func() {
		dur := config.RetentionPeriod.Duration
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(dur):
				now := time.Now().UTC()
				before := now.Add(-dur)
				purged, err := components_metrics_state.PurgeMetrics(ctx, dbRW, components_metrics_state.DefaultTableName, before)
				if err != nil {
					log.Logger.Warnw("failed to purge metrics", "error", err)
				} else {
					log.Logger.Debugw("purged metrics", "purged", purged)
				}
			}
		}
	}()

	allComponents := make([]apiv1.Component, 0)
	if _, ok := config.Components[os.Name]; !ok {
		allComponents = append(allComponents, os.New(ctx, rebootEventStore))
	}

	for k, configValue := range config.Components {
		switch k {
		case cpu.Name:
			c, err := cpu.New(ctx, eventStore)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case disk.Name:
			allComponents = append(allComponents, disk.New(
				ctx,
				[]string{"/"},
				[]string{"/var/lib/kubelet"},
			))

		case fuse.Name:
			c, err := fuse.New(ctx, fuse.DefaultCongestedPercentAgainstThreshold, fuse.DefaultMaxBackgroundPercentAgainstThreshold, eventStore)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case pci.Name:
			c, err := pci.New(ctx, eventStore)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case fd.Name:
			c, err := fd.New(ctx, eventStore)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case kernel_module.Name:
			kernelModulesToCheck := []string{}
			if configValue != nil {
				var ok bool
				kernelModulesToCheck, ok = configValue.([]string)
				if !ok {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
			}
			allComponents = append(allComponents, kernel_module.New(kernelModulesToCheck))

		case library.Name:
			if configValue != nil {
				libCfg, ok := configValue.(library.Config)
				if !ok {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				allComponents = append(allComponents, library.New(libCfg))
			}

		case info.Name:
			allComponents = append(allComponents, info.New(config.Annotations, dbRO))

		case memory.Name:
			c, err := memory.New(ctx, eventStore)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case os.Name:
			allComponents = append(allComponents, os.New(ctx, rebootEventStore))

		case tailscale.Name:
			allComponents = append(allComponents, tailscale.New(ctx))

		case nvidia_info.Name:
			allComponents = append(allComponents, nvidia_info.New(ctx, nvmlInstanceV2))

		case nvidia_badenvs.Name:
			allComponents = append(allComponents, nvidia_badenvs.New(ctx))

		case nvidia_xid.Name:
			allComponents = append(allComponents, nvidia_xid.New(ctx, rebootEventStore, eventStore))

		case nvidia_sxid.Name:
			// db object to read sxid events (read-only, writes are done in poller)
			allComponents = append(allComponents, nvidia_sxid.New(ctx, rebootEventStore, eventStore))

		case nvidia_hw_slowdown.Name:
			c, err := nvidia_hw_slowdown.New(ctx, nvmlInstanceV2, eventStore)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_clock_speed.Name:
			allComponents = append(allComponents, nvidia_clock_speed.New(ctx, nvmlInstanceV2))

		case nvidia_ecc.Name:
			allComponents = append(allComponents, nvidia_ecc.New(ctx, nvmlInstanceV2))

		case nvidia_memory.Name:
			allComponents = append(allComponents, nvidia_memory.New(ctx, nvmlInstanceV2))

		case nvidia_gpm.Name:
			allComponents = append(allComponents, nvidia_gpm.New(ctx, nvmlInstanceV2))

		case nvidia_nvlink.Name:
			allComponents = append(allComponents, nvidia_nvlink.New(ctx, nvmlInstanceV2))

		case nvidia_power.Name:
			allComponents = append(allComponents, nvidia_power.New(ctx, nvmlInstanceV2))

		case nvidia_temperature.Name:
			allComponents = append(allComponents, nvidia_temperature.New(ctx, nvmlInstanceV2))

		case nvidia_utilization.Name:
			allComponents = append(allComponents, nvidia_utilization.New(ctx, nvmlInstanceV2))

		case nvidia_processes.Name:
			allComponents = append(allComponents, nvidia_processes.New(ctx, nvmlInstanceV2))

		case nvidia_remapped_rows.Name:
			c, err := nvidia_remapped_rows.New(ctx, nvmlInstanceV2, eventStore)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_fabric_manager.Name:
			fabricManagerLogComponent, err := nvidia_fabric_manager.New(ctx, eventStore)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, fabricManagerLogComponent)

		case nvidia_gsp_firmware_mode.Name:
			allComponents = append(allComponents, nvidia_gsp_firmware_mode.New(ctx, nvmlInstanceV2))

		case nvidia_infiniband.Name:
			c, err := nvidia_infiniband.New(ctx, eventStore, config.NvidiaToolOverwrites)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_peermem.Name:
			c, err := nvidia_peermem.New(ctx, eventStore)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_persistence_mode.Name:
			allComponents = append(allComponents, nvidia_persistence_mode.New(ctx, nvmlInstanceV2))

		case nvidia_nccl.Name:
			c, err := nvidia_nccl.New(ctx, eventStore)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case containerd_pod.Name:
			allComponents = append(allComponents, containerd_pod.New(ctx))

		case docker_container.Name:
			allComponents = append(allComponents, docker_container.New(ctx, config.DockerIgnoreConnectionErrors))

		case kubelet_pod.Name:
			allComponents = append(allComponents, kubelet_pod.New(ctx, kubelet_pod.DefaultKubeletReadOnlyPort))

		case network_latency.Name:
			allComponents = append(allComponents, network_latency.New(ctx))

		default:
			return nil, fmt.Errorf("unknown component %s", k)
		}
	}

	go func() {
		ticker := time.NewTicker(time.Minute) // only first run is 1-minute wait
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ticker.Reset(20 * time.Minute)
			}

			total, err := metrics.ReadRegisteredTotal(pkgmetrics.DefaultGatherer())
			if err != nil {
				log.Logger.Errorw("failed to get registered total", "error", err)
				continue
			}

			log.Logger.Debugw("components status",
				"inflight_components", total,
			)
		}
	}()

	// track metrics every hour
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ticker.Reset(time.Hour)
			}

			if err := gpud_state.RecordMetrics(ctx, dbRW); err != nil {
				log.Logger.Errorw("failed to record metrics", "error", err)
			}
		}
	}()

	// compact the state database every retention period
	if config.CompactPeriod.Duration > 0 {
		go func() {
			ticker := time.NewTicker(config.CompactPeriod.Duration)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					ticker.Reset(config.CompactPeriod.Duration)
				}

				if err := sqlite.Compact(ctx, dbRW); err != nil {
					log.Logger.Errorw("failed to compact state database", "error", err)
				}
			}
		}()
	} else {
		log.Logger.Debugw("compact period is not set, skipping compacting")
	}

	for i := range allComponents {
		metrics.SetRegistered(allComponents[i].Name())
	}

	var componentNames []string
	componentSet := make(map[string]struct{})
	for _, c := range allComponents {
		componentSet[c.Name()] = struct{}{}
		componentNames = append(componentNames, c.Name())

		// this guarantees no name conflict, thus safe to register handlers by its name
		if err := components.RegisterComponent(c.Name(), c); err != nil {
			log.Logger.Debugw("failed to register component", "name", c.Name(), "error", err)
			continue
		}
	}

	for _, c := range allComponents {
		if err = c.Start(); err != nil {
			log.Logger.Errorw("failed to start component", "name", c.Name(), "error", err)
			return nil, fmt.Errorf("failed to start component %s: %w", c.Name(), err)
		}
	}

	uid, err := gpud_state.CreateMachineIDIfNotExist(ctx, dbRW, dbRO, cliUID)
	if err != nil {
		return nil, fmt.Errorf("failed to create machine uid: %w", err)
	}
	s.uid = uid
	if err = gpud_state.UpdateComponents(ctx, dbRW, uid, strings.Join(componentNames, ",")); err != nil {
		return nil, fmt.Errorf("failed to update components: %w", err)
	}

	// TODO: implement configuration file refresh + apply

	router := gin.Default()
	router.SetHTMLTemplate(rootTmpl)

	cert, err := s.generateSelfSignedCert()
	if err != nil {
		return nil, fmt.Errorf("failed to generate tls cert: %w", err)
	}

	installRootGinMiddlewares(router)
	installCommonGinMiddlewares(router, log.Logger.Desugar())

	v1 := router.Group("/v1")

	// if the request header is set "Accept-Encoding: gzip",
	// the middleware automatically gzip-compresses the response with the response header "Content-Encoding: gzip"
	v1.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/update/"})))

	ghler := newGlobalHandler(config, components.GetAllComponents(), metricsSQLiteStore)
	registeredPaths := ghler.registerComponentRoutes(v1)
	for i := range registeredPaths {
		registeredPaths[i].Path = path.Join(v1.BasePath(), registeredPaths[i].Path)
	}

	registeredPaths = append(registeredPaths, componentHandlerDescription{
		Path: "/metrics",
		Desc: "Prometheus metrics",
	})
	promHandler := promhttp.HandlerFor(pkgmetrics.DefaultGatherer(), promhttp.HandlerOpts{})
	router.GET("/metrics", func(ctx *gin.Context) {
		promHandler.ServeHTTP(ctx.Writer, ctx.Request)
	})

	router.GET(URLPathSwagger, ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET(URLPathHealthz, createHealthzHandler())
	registeredPaths = append(registeredPaths, componentHandlerDescription{
		Path: URLPathHealthz,
		Desc: URLPathHealthzDesc,
	})

	admin := router.Group(urlPathAdmin)

	admin.GET(URLPathConfig, createConfigHandler(config))
	registeredPaths = append(registeredPaths, componentHandlerDescription{
		Path: path.Join("/admin", URLPathConfig),
		Desc: URLPathConfigDesc,
	})
	admin.GET(urlPathPackages, createPackageHandler(packageManager))
	registeredPaths = append(registeredPaths, componentHandlerDescription{
		Path: URLPathAdminPackages,
		Desc: urlPathPackagesDesc,
	})

	if config.Pprof {
		log.Logger.Debugw("registering pprof handlers")
		admin.GET("/pprof/profile", gin.WrapH(http.HandlerFunc(pprof.Profile)))
		admin.GET("/pprof/heap", gin.WrapH(pprof.Handler("heap")))
		admin.GET("/pprof/trace", gin.WrapH(http.HandlerFunc(pprof.Trace)))
	}

	if config.Web != nil && config.Web.Enable {
		router.GET("/", createRootHandler(registeredPaths, *config.Web))

		if config.Web.Enable {
			go func() {
				time.Sleep(2 * time.Second)
				url := "https://" + config.Address
				if !strings.HasPrefix(config.Address, "127.0.0.1") && !strings.HasPrefix(config.Address, "0.0.0.0") && !strings.HasPrefix(config.Address, "localhost") {
					url = "https://localhost" + config.Address
				}
				fmt.Printf("\n\n\n\n\n%s serving %s\n\n\n\n\n", checkMark, url)
			}()
		}
	}

	go s.updateToken(ctx, dbRW, uid, endpoint, metricsSQLiteStore)

	go func(nvmlInstance nvidia_query_nvml.InstanceV2, metricsSyncer *pkgmetricssyncer.Syncer) {
		defer func() {
			if nvmlInstance != nil {
				if err := nvmlInstance.Shutdown(); err != nil {
					log.Logger.Warnw("failed to shutdown NVML instance", "error", err)
				}
			}
			if metricsSyncer != nil {
				metricsSyncer.Stop()
			}
		}()

		srv := &http.Server{
			Addr:    config.Address,
			Handler: router,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		}
		log.Logger.Infof("serving %s", config.Address)

		// Start HTTPS server
		err = srv.ListenAndServeTLS("", "")
		if err != nil {
			s.Stop()
			log.Logger.Fatalf("serve %v failure %v", config.Address, err)
		}
	}(nvmlInstanceV2, syncer)

	ghler.componentNamesMu.RLock()
	currComponents := ghler.componentNames
	ghler.componentNamesMu.RUnlock()
	if err = login.Gossip(uid, endpoint, currComponents); err != nil {
		log.Logger.Debugf("failed to gossip: %v", err)
	}
	return s, nil
}

const checkMark = "\033[32m✔\033[0m"

func (s *Server) Stop() {
	if s.session != nil {
		s.session.Stop()
	}
	for name, component := range components.GetAllComponents() {
		closer, ok := component.(io.Closer)
		if !ok {
			continue
		}
		if err := closer.Close(); err != nil {
			log.Logger.Errorf("failed to close plugin %v: %v", name, err)
		}
	}

	if cerr := s.dbRW.Close(); cerr != nil {
		log.Logger.Debugw("failed to close read-write db", "error", cerr)
	} else {
		log.Logger.Debugw("successfully closed read-write db")
	}
	if cerr := s.dbRO.Close(); cerr != nil {
		log.Logger.Debugw("failed to close read-only db", "error", cerr)
	} else {
		log.Logger.Debugw("successfully closed read-only db")
	}

	if s.fifo != nil {
		if err := s.fifo.Close(); err != nil {
			log.Logger.Errorf("failed to close fifo: %v", err)
		}
	}
	if s.fifoPath != "" {
		if err := stdos.Remove(s.fifoPath); err != nil {
			log.Logger.Errorf("failed to remove fifo: %s", err)
		}
	}
}

func (s *Server) generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Create a certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Lepton AI"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Create the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Encode the certificate and private key to PEM format
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})

	// Load the certificate
	cert, err := tls.X509KeyPair(certPEM, privPEM)
	if err != nil {
		return tls.Certificate{}, err
	}

	return cert, nil
}

func (s *Server) updateToken(ctx context.Context, db *sql.DB, uid string, endpoint string, metricsStore pkgmetrics.Store) {
	var userToken string
	pipePath := s.fifoPath
	if dbToken, err := gpud_state.GetLoginInfo(ctx, db, uid); err == nil {
		userToken = dbToken
	}

	if userToken != "" {
		var err error
		s.session, err = session.NewSession(
			ctx,
			fmt.Sprintf("https://%s/api/v1/session", endpoint),
			session.WithMachineID(uid),
			session.WithPipeInterval(3*time.Second),
			session.WithEnableAutoUpdate(s.enableAutoUpdate),
			session.WithAutoUpdateExitCode(s.autoUpdateExitCode),
			session.WithMetricsStore(metricsStore),
		)
		if err != nil {
			log.Logger.Errorw("error creating session", "error", err)
		}
	}

	if _, err := stdos.Stat(pipePath); err == nil {
		if err = stdos.Remove(pipePath); err != nil {
			log.Logger.Errorf("error creating pipe: %v", err)
			return
		}
	} else if !stdos.IsNotExist(err) {
		log.Logger.Errorf("error stat pipe: %v", err)
		return
	}

	if err := syscall.Mkfifo(pipePath, 0666); err != nil {
		log.Logger.Errorf("error creating pipe: %v", err)
		return
	}
	for {
		pipe, err := stdos.OpenFile(pipePath, stdos.O_RDONLY, stdos.ModeNamedPipe)
		if err != nil {
			log.Logger.Errorf("error opening named pipe: %v", err)
			return
		}
		buffer := make([]byte, 1024)
		if n, err := pipe.Read(buffer); err != nil {
			log.Logger.Errorf("error reading pipe: %v", err)
		} else {
			userToken = string(buffer[:n])
		}

		pipe.Close()
		if userToken != "" {
			if s.session != nil {
				s.session.Stop()
			}
			s.session, err = session.NewSession(
				ctx,
				fmt.Sprintf("https://%s/api/v1/session", endpoint),
				session.WithMachineID(uid),
				session.WithPipeInterval(3*time.Second),
				session.WithEnableAutoUpdate(s.enableAutoUpdate),
				session.WithAutoUpdateExitCode(s.autoUpdateExitCode),
				session.WithMetricsStore(metricsStore),
			)
			if err != nil {
				log.Logger.Errorw("error creating session", "error", err)
			}
		}

		time.Sleep(time.Second)
	}
}

func WriteToken(token string, fifoFile string) error {
	var f *stdos.File
	var err error
	for i := 0; i < 30; i++ {
		if _, err = stdos.Stat(fifoFile); stdos.IsNotExist(err) {
			time.Sleep(1 * time.Second)
			continue
		} else if err != nil {
			return fmt.Errorf("failed to stat fifo file: %w", err)
		}
	}
	if err != nil {
		return fmt.Errorf("server not ready")
	}

	if f, err = stdos.OpenFile(fifoFile, stdos.O_WRONLY, 0600); err != nil {
		return fmt.Errorf("failed to open fifo file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Logger.Errorf("failed to close token fifo: %v", err)
		}
	}()

	_, err = f.Write([]byte(token))
	if err != nil {
		return fmt.Errorf("failed to write token: %w", err)
	}
	return nil
}
