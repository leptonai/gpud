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
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/pprof"
	"net/url"
	stdos "os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	_ "github.com/leptonai/gpud/docs/apis"
	lepconfig "github.com/leptonai/gpud/pkg/config"
	customplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/gossip"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	gpudstate "github.com/leptonai/gpud/pkg/gpud-state"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	pkgmetricsscraper "github.com/leptonai/gpud/pkg/metrics/scraper"
	pkgmetricsstore "github.com/leptonai/gpud/pkg/metrics/store"
	pkgmetricssyncer "github.com/leptonai/gpud/pkg/metrics/syncer"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/session"
	"github.com/leptonai/gpud/pkg/sqlite"

	componentsacceleratornvidiabadenvs "github.com/leptonai/gpud/components/accelerator/nvidia/bad-envs"
	componentsacceleratornvidiaclockspeed "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed"
	componentsacceleratornvidiaecc "github.com/leptonai/gpud/components/accelerator/nvidia/ecc"
	componentsacceleratornvidiafabricmanager "github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager"
	componentsacceleratornvidiagpm "github.com/leptonai/gpud/components/accelerator/nvidia/gpm"
	componentsacceleratornvidiagspfirmwaremode "github.com/leptonai/gpud/components/accelerator/nvidia/gsp-firmware-mode"
	componentsacceleratornvidiahwslowdown "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown"
	componentsacceleratornvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	componentsacceleratornvidiainfo "github.com/leptonai/gpud/components/accelerator/nvidia/info"
	componentsacceleratornvidiamemory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	componentsacceleratornvidianccl "github.com/leptonai/gpud/components/accelerator/nvidia/nccl"
	componentsacceleratornvidianvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	componentsacceleratornvidiapeermem "github.com/leptonai/gpud/components/accelerator/nvidia/peermem"
	componentsacceleratornvidiapersistencemode "github.com/leptonai/gpud/components/accelerator/nvidia/persistence-mode"
	componentsacceleratornvidiapower "github.com/leptonai/gpud/components/accelerator/nvidia/power"
	componentsacceleratornvidiaprocesses "github.com/leptonai/gpud/components/accelerator/nvidia/processes"
	componentsacceleratornvidiaremappedrows "github.com/leptonai/gpud/components/accelerator/nvidia/remapped-rows"
	componentsacceleratornvidiasxid "github.com/leptonai/gpud/components/accelerator/nvidia/sxid"
	componentsacceleratornvidiatemperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	componentsacceleratornvidiautilization "github.com/leptonai/gpud/components/accelerator/nvidia/utilization"
	componentsacceleratornvidiaxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	componentscontainerdpod "github.com/leptonai/gpud/components/containerd/pod"
	componentscpu "github.com/leptonai/gpud/components/cpu"
	componentsdisk "github.com/leptonai/gpud/components/disk"
	componentsdockercontainer "github.com/leptonai/gpud/components/docker/container"
	componentsfd "github.com/leptonai/gpud/components/fd"
	componentsfuse "github.com/leptonai/gpud/components/fuse"
	componentsinfo "github.com/leptonai/gpud/components/info"
	componentskernelmodule "github.com/leptonai/gpud/components/kernel-module"
	componentskubeletpod "github.com/leptonai/gpud/components/kubelet/pod"
	componentslibrary "github.com/leptonai/gpud/components/library"
	componentsmemory "github.com/leptonai/gpud/components/memory"
	componentsnetworklatency "github.com/leptonai/gpud/components/network/latency"
	componentsos "github.com/leptonai/gpud/components/os"
	componentspci "github.com/leptonai/gpud/components/pci"
	componentstailscale "github.com/leptonai/gpud/components/tailscale"
)

var componentInits = []components.InitFunc{
	componentscpu.New,
	componentscontainerdpod.New,
	componentsdisk.New,
	componentsdockercontainer.New,
	componentsfd.New,
	componentsfuse.New,
	componentsinfo.New,
	componentskernelmodule.New,
	componentskubeletpod.New,
	componentslibrary.New,
	componentsmemory.New,
	componentsnetworklatency.New,
	componentsos.New,
	componentspci.New,
	componentstailscale.New,
	componentsacceleratornvidiabadenvs.New,
	componentsacceleratornvidiaclockspeed.New,
	componentsacceleratornvidiaecc.New,
	componentsacceleratornvidiafabricmanager.New,
	componentsacceleratornvidiagpm.New,
	componentsacceleratornvidiagspfirmwaremode.New,
	componentsacceleratornvidiahwslowdown.New,
	componentsacceleratornvidiainfiniband.New,
	componentsacceleratornvidiainfo.New,
	componentsacceleratornvidiamemory.New,
	componentsacceleratornvidianccl.New,
	componentsacceleratornvidianvlink.New,
	componentsacceleratornvidiapeermem.New,
	componentsacceleratornvidiapersistencemode.New,
	componentsacceleratornvidiapower.New,
	componentsacceleratornvidiaprocesses.New,
	componentsacceleratornvidiaremappedrows.New,
	componentsacceleratornvidiasxid.New,
	componentsacceleratornvidiatemperature.New,
	componentsacceleratornvidiautilization.New,
	componentsacceleratornvidiaxid.New,
}

// Server is the gpud main daemon
type Server struct {
	dbRW *sql.DB
	dbRO *sql.DB

	// initRegistry is the registry for init plugins
	// that runs before the regular components
	// e.g., install python, ...
	// in most cases, this runs only once
	// to avoid conflicting with other periodic checks
	initRegistry components.Registry

	// componentsRegistry is the registry for the regular components
	componentsRegistry components.Registry

	machineID string

	fifoPath string
	fifo     *stdos.File

	session *session.Session

	enableAutoUpdate   bool
	autoUpdateExitCode int
}

type UserToken struct {
	userToken string
	mu        sync.RWMutex
}

func createURL(endpoint string) string {
	host := endpoint
	url, err := url.Parse(endpoint)
	if err == nil && url != nil && url.Host != "" {
		host = url.Host
	}
	return fmt.Sprintf("https://%s", host)
}

func New(ctx context.Context, config *lepconfig.Config, endpoint string, packageManager *gpudmanager.Manager) (_ *Server, retErr error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}
	endpoint = createURL(endpoint)

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

	// only record once when we create the server instance
	cctx, ccancel := context.WithTimeout(ctx, time.Minute)
	err = rebootEventStore.RecordReboot(cctx)
	ccancel()
	if err != nil {
		log.Logger.Errorw("failed to record reboot", "error", err)
	}

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

		fifoPath: fifoPath,

		enableAutoUpdate:   config.EnableAutoUpdate,
		autoUpdateExitCode: config.AutoUpdateExitCode,
	}
	defer func() {
		if retErr != nil {
			s.Stop()
		}
	}()

	nvmlInstance, err := nvidianvml.NewWithExitOnSuccessfulLoad(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create NVML instance: %w", err)
	}

	if err := gpudstate.CreateTableMachineMetadata(ctx, dbRW); err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}
	if err := gpudstate.CreateTableAPIVersion(ctx, dbRW); err != nil {
		return nil, fmt.Errorf("failed to create api version table: %w", err)
	}
	ver, err := gpudstate.UpdateAPIVersionIfNotExists(ctx, dbRW, "v1")
	if err != nil {
		return nil, fmt.Errorf("failed to update api version: %w", err)
	}
	log.Logger.Infow("api version", "version", ver)
	if ver != "v1" {
		return nil, fmt.Errorf("api version mismatch: %s (only supports v1)", ver)
	}

	gpudInstance := &components.GPUdInstance{
		RootCtx: ctx,

		NVMLInstance:         nvmlInstance,
		NVIDIAToolOverwrites: config.NvidiaToolOverwrites,

		Annotations: config.Annotations,
		DBRO:        dbRO,

		EventStore:       eventStore,
		RebootEventStore: rebootEventStore,

		MountPoints:  []string{"/"},
		MountTargets: []string{"/var/lib/kubelet"},
	}

	s.componentsRegistry = components.NewRegistry(gpudInstance)
	for _, initFunc := range componentInits {
		s.componentsRegistry.MustRegister(initFunc)
	}

	// must be registered before starting the components
	s.initRegistry = components.NewRegistry(gpudInstance)
	if config.PluginSpecsFile != "" {
		specs, err := customplugins.LoadSpecs(config.PluginSpecsFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load plugin specs: %w", err)
		}

		if err := specs.Validate(); err != nil {
			return nil, fmt.Errorf("failed to validate plugin specs: %w", err)
		}

		for _, spec := range specs {
			initFunc := spec.NewInitFunc()
			if initFunc == nil {
				log.Logger.Errorw("failed to load plugin", "name", spec.ComponentName())
				continue
			}

			if spec.Type == customplugins.SpecTypeInit {
				s.initRegistry.MustRegister(initFunc)
				log.Logger.Infow("loaded init plugin", "name", spec.ComponentName())
			} else {
				s.componentsRegistry.MustRegister(initFunc)
				log.Logger.Infow("loaded component plugin", "name", spec.ComponentName())
			}
		}
	}

	// init plugin run only "once", and "before" regular components
	// thus no need to start
	for _, c := range s.initRegistry.All() {
		rs := c.Check()
		if rs.HealthStateType() != apiv1.HealthStateTypeHealthy {
			return nil, fmt.Errorf("failed to start init plugin %s: %s", c.Name(), rs.Summary())
		}
		log.Logger.Infow("successfully executed init plugin", "name", c.Name(), "summary", rs.Summary())

		debugger, ok := rs.(components.CheckResultDebugger)
		if ok {
			fmt.Printf("init plugin debug output %q:\n\n%s\n\n", c.Name(), debugger.Debug())
		}
	}

	componentNames := make([]string, 0)
	for _, c := range s.componentsRegistry.All() {
		if err = c.Start(); err != nil {
			return nil, fmt.Errorf("failed to start component %s: %w", c.Name(), err)
		}
		componentNames = append(componentNames, c.Name())
	}

	go recordInternalMetrics(ctx, dbRW)
	go doCompact(ctx, dbRW, config.CompactPeriod.Duration)

	s.machineID, err = gpudstate.ReadMachineID(ctx, dbRO)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to read machine uid: %w", err)
	}
	if s.machineID != "" {
		if err = gpudstate.UpdateComponents(ctx, dbRW, s.machineID, strings.Join(componentNames, ",")); err != nil {
			return nil, fmt.Errorf("failed to update components: %w", err)
		}
	}

	cert, err := s.generateSelfSignedCert()
	if err != nil {
		return nil, fmt.Errorf("failed to generate tls cert: %w", err)
	}

	router := gin.Default()
	installRootGinMiddlewares(router)
	installCommonGinMiddlewares(router, log.Logger.Desugar())

	globalHandler := newGlobalHandler(config, s.componentsRegistry, metricsSQLiteStore, gpudInstance)

	// if the request header is set "Accept-Encoding: gzip",
	// the middleware automatically gzip-compresses the response with the response header "Content-Encoding: gzip"
	v1Group := router.Group("/v1")
	v1Group.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/update/"})))
	globalHandler.registerComponentRoutes(v1Group)

	promHandler := promhttp.HandlerFor(pkgmetrics.DefaultGatherer(), promhttp.HandlerOpts{})
	router.GET("/metrics", func(ctx *gin.Context) {
		promHandler.ServeHTTP(ctx.Writer, ctx.Request)
	})

	router.GET(URLPathSwagger, ginswagger.WrapHandler(swaggerfiles.Handler))
	router.GET(URLPathHealthz, handleHealthz())
	router.GET(URLPathMachineInfo, globalHandler.handleMachineInfo)

	adminGroup := router.Group(urlPathAdmin)
	adminGroup.GET(urlPathConfig, handleAdminConfig(config))
	adminGroup.GET(urlPathPackages, handleAdminPackagesStatus(packageManager))

	if config.Pprof {
		log.Logger.Debugw("registering pprof handlers")
		adminGroup.GET("/pprof/profile", gin.WrapH(http.HandlerFunc(pprof.Profile)))
		adminGroup.GET("/pprof/heap", gin.WrapH(pprof.Handler("heap")))
		adminGroup.GET("/pprof/trace", gin.WrapH(http.HandlerFunc(pprof.Trace)))
	}

	userToken := &UserToken{}
	go s.updateToken(ctx, dbRW, endpoint, metricsSQLiteStore, userToken)
	go s.sendGossip(ctx, nvmlInstance, endpoint, userToken)
	go s.startListener(nvmlInstance, syncer, config, router, cert)

	return s, nil
}

func (s *Server) Stop() {
	if s.session != nil {
		s.session.Stop()
	}

	if s.componentsRegistry != nil {
		for _, component := range s.componentsRegistry.All() {
			closer, ok := component.(io.Closer)
			if !ok {
				continue
			}
			if err := closer.Close(); err != nil {
				log.Logger.Errorf("failed to close plugin %v: %v", component.Name(), err)
			}
		}
	}

	if s.dbRW != nil {
		if cerr := s.dbRW.Close(); cerr != nil {
			log.Logger.Debugw("failed to close read-write db", "error", cerr)
		} else {
			log.Logger.Debugw("successfully closed read-write db")
		}
	}
	if s.dbRO != nil {
		if cerr := s.dbRO.Close(); cerr != nil {
			log.Logger.Debugw("failed to close read-only db", "error", cerr)
		} else {
			log.Logger.Debugw("successfully closed read-only db")
		}
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

func (s *Server) updateToken(ctx context.Context, db *sql.DB, endpoint string, metricsStore pkgmetrics.Store, token *UserToken) {
	var userToken string
	pipePath := s.fifoPath
	if dbToken, err := gpudstate.GetLoginInfo(ctx, db, s.machineID); err == nil {
		userToken = dbToken

		token.mu.Lock()
		token.userToken = userToken
		token.mu.Unlock()
	}

	if userToken != "" {
		var err error
		s.session, err = session.NewSession(
			ctx,
			endpoint,
			userToken,
			session.WithMachineID(s.machineID),
			session.WithPipeInterval(3*time.Second),
			session.WithEnableAutoUpdate(s.enableAutoUpdate),
			session.WithAutoUpdateExitCode(s.autoUpdateExitCode),
			session.WithComponentsRegistry(s.componentsRegistry),
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
			token.mu.Lock()
			token.userToken = userToken
			token.mu.Unlock()
			if s.session != nil {
				s.session.Stop()
			}
			s.session, err = session.NewSession(
				ctx,
				endpoint,
				userToken,
				session.WithMachineID(s.machineID),
				session.WithPipeInterval(3*time.Second),
				session.WithEnableAutoUpdate(s.enableAutoUpdate),
				session.WithAutoUpdateExitCode(s.autoUpdateExitCode),
				session.WithComponentsRegistry(s.componentsRegistry),
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
	var err error
	for i := 0; i < 10; i++ {
		if _, err = stdos.Stat(fifoFile); stdos.IsNotExist(err) {
			time.Sleep(1 * time.Second)
			continue
		} else if err != nil {
			return fmt.Errorf("failed to stat fifo file: %w", err)
		}
	}
	if err != nil {
		return errors.New("server not ready")
	}

	var f *stdos.File
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

func recordInternalMetrics(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ticker.Reset(time.Hour)
		}

		if err := gpudstate.RecordMetrics(ctx, db); err != nil {
			log.Logger.Errorw("failed to record metrics", "error", err)
		}
	}
}

func doCompact(ctx context.Context, db *sql.DB, compactPeriod time.Duration) {
	if compactPeriod <= 0 {
		log.Logger.Debugw("compact period is not set, skipping compacting")
		return
	}

	ticker := time.NewTicker(compactPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ticker.Reset(compactPeriod)
		}

		if err := sqlite.Compact(ctx, db); err != nil {
			log.Logger.Errorw("failed to compact state database", "error", err)
		}
	}
}

func (s *Server) sendGossip(ctx context.Context, nvmlInstance nvidianvml.Instance, endpoint string, userToken *UserToken) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	var token string
	for {
		userToken.mu.RLock()
		token = userToken.userToken
		userToken.mu.RUnlock()
		if token == "" {
			continue
		}
		gossipReq, err := pkgmachineinfo.CreateGossipRequest(s.machineID, nvmlInstance, token)
		if err != nil {
			log.Logger.Errorw("failed to create gossip request", "error", err)
		} else {
			if _, err = gossip.SendRequest(ctx, endpoint, *gossipReq); err != nil {
				log.Logger.Errorw("failed to gossip", "error", err)
			} else {
				log.Logger.Debugw("successfully sent gossip")
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) startListener(nvmlInstance nvidianvml.Instance, metricsSyncer *pkgmetricssyncer.Syncer, config *lepconfig.Config, router *gin.Engine, cert tls.Certificate) {
	defer func() {
		if nvmlInstance != nil {
			if err := nvmlInstance.Shutdown(); err != nil {
				log.Logger.Warnw("failed to shutdown NVML instance", "error", err)
			}
		}

		if metricsSyncer != nil {
			metricsSyncer.Stop()
		}

		s.Stop()
	}()

	log.Logger.Infow("gpud started serving", "address", config.Address, "pluginSpecFile", config.PluginSpecsFile)

	srv := &http.Server{
		Addr:    config.Address,
		Handler: router,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}
	if err := srv.ListenAndServeTLS("", ""); err != nil {
		log.Logger.Warnw("gpud serve failed", "address", config.Address, "error", err)
		stdos.Exit(1)
	}
}
