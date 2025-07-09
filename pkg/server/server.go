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
	"github.com/leptonai/gpud/components/all"
	_ "github.com/leptonai/gpud/docs/apis"
	lepconfig "github.com/leptonai/gpud/pkg/config"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/httputil"
	pkgkmsgwriter "github.com/leptonai/gpud/pkg/kmsg/writer"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
	pkgmetricsscraper "github.com/leptonai/gpud/pkg/metrics/scraper"
	pkgmetricsstore "github.com/leptonai/gpud/pkg/metrics/store"
	pkgmetricssyncer "github.com/leptonai/gpud/pkg/metrics/syncer"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/session"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// Server is the gpud main daemon
type Server struct {
	auditLogger log.AuditLogger

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

	machineIDMu sync.RWMutex
	machineID   string

	// epLocalGPUdServer is the endpoint of the local GPUd server
	epLocalGPUdServer string
	// epControlPlane is the endpoint of the control plane
	epControlPlane string

	fifoPath string
	fifo     *stdos.File

	gpudInstance *components.GPUdInstance
	session      *session.Session

	enableAutoUpdate   bool
	autoUpdateExitCode int

	pluginSpecsFile string
	faultInjector   pkgfaultinjector.Injector
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

func New(ctx context.Context, auditLogger log.AuditLogger, config *lepconfig.Config, packageManager *gpudmanager.Manager) (_ *Server, retErr error) {
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

	if err := pkgmetadata.CreateTableMetadata(ctx, dbRW); err != nil {
		return nil, fmt.Errorf("failed to create metadata table: %w", err)
	}

	// by default, we only retain past 24 hours of events
	eventStore, err := eventstore.New(dbRW, dbRO, 24*time.Hour)
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
	syncer := pkgmetricssyncer.NewSyncer(ctx, promScraper, metricsSQLiteStore, time.Minute, time.Minute, 24*time.Hour)
	syncer.Start()

	promRecorder := pkgmetricsrecorder.NewPrometheusRecorder(ctx, 15*time.Minute, dbRO)
	promRecorder.Start()

	fifoPath, err := lepconfig.DefaultFifoFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get fifo path: %w", err)
	}
	s := &Server{
		auditLogger: auditLogger,

		dbRW: dbRW,
		dbRO: dbRO,

		fifoPath: fifoPath,

		enableAutoUpdate:   config.EnableAutoUpdate,
		autoUpdateExitCode: config.AutoUpdateExitCode,

		pluginSpecsFile: config.PluginSpecsFile,
	}
	defer func() {
		if retErr != nil {
			s.Stop()
		}
	}()

	s.machineID, err = pkgmetadata.ReadMachineIDWithFallback(ctx, dbRW, dbRO)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to read machine uid: %w", err)
	}

	kmsgWriter := pkgkmsgwriter.NewWriter(pkgkmsgwriter.DefaultDevKmsg)
	s.faultInjector = pkgfaultinjector.NewInjector(kmsgWriter)

	nvmlInstance, err := nvidianvml.NewWithExitOnSuccessfulLoad(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create NVML instance: %w", err)
	}

	s.gpudInstance = &components.GPUdInstance{
		RootCtx: ctx,

		MachineID: s.machineID,

		NVMLInstance:         nvmlInstance,
		NVIDIAToolOverwrites: config.NvidiaToolOverwrites,

		DBRW: dbRW,
		DBRO: dbRO,

		EventStore:       eventStore,
		RebootEventStore: rebootEventStore,

		MountPoints:  []string{"/"},
		MountTargets: []string{"/var/lib/kubelet"},
	}
	if s.gpudInstance.MachineID == "" {
		s.gpudInstance.MachineID = pkghost.MachineID()
		log.Logger.Infow("assigned machine id not found, using host level machine ID", "machineID", s.gpudInstance.MachineID)
	}

	s.componentsRegistry = components.NewRegistry(s.gpudInstance)
	for _, c := range all.All() {
		name := c.Name

		shouldEnable := config.ShouldEnable(name)
		if config.ShouldDisable(name) {
			shouldEnable = false
		}

		if shouldEnable {
			s.componentsRegistry.MustRegister(c.InitFunc)
		}
	}

	// must be registered before starting the components
	s.initRegistry = components.NewRegistry(s.gpudInstance)
	if config.PluginSpecsFile != "" {
		_, err := stdos.Stat(config.PluginSpecsFile)
		exists := err == nil

		if exists {
			specs, err := pkgcustomplugins.LoadSpecs(config.PluginSpecsFile)
			if err != nil {
				return nil, fmt.Errorf("failed to load plugin specs: %w", err)
			}

			for _, spec := range specs {
				initFunc := spec.NewInitFunc()
				if initFunc == nil {
					log.Logger.Errorw("failed to load plugin", "name", spec.ComponentName())
					continue
				}

				if spec.PluginType == pkgcustomplugins.SpecTypeInit {
					s.initRegistry.MustRegister(initFunc)
					log.Logger.Infow("loaded init plugin", "name", spec.ComponentName())
				} else {
					s.componentsRegistry.MustRegister(initFunc)
					log.Logger.Infow("loaded component plugin", "name", spec.ComponentName())
				}
			}
		} else {
			log.Logger.Warnw("plugin specs file does not exist, skipping", "path", config.PluginSpecsFile)
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

	// component must be started after initialization
	for _, c := range s.componentsRegistry.All() {
		if err = c.Start(); err != nil {
			return nil, fmt.Errorf("failed to start component %s: %w", c.Name(), err)
		}
	}
	go doCompact(ctx, dbRW, config.CompactPeriod.Duration)

	cert, err := s.generateSelfSignedCert()
	if err != nil {
		return nil, fmt.Errorf("failed to generate tls cert: %w", err)
	}

	router := gin.Default()
	installRootGinMiddlewares(router)
	installCommonGinMiddlewares(router, log.Logger.Desugar())

	globalHandler := newGlobalHandler(config, s.componentsRegistry, metricsSQLiteStore, s.gpudInstance, s.faultInjector)

	// if the request header is set "Accept-Encoding: gzip",
	// the middleware automatically gzip-compresses the response with the response header "Content-Encoding: gzip"
	v1Group := router.Group("/v1")
	v1Group.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/update/"})))
	globalHandler.registerComponentRoutes(v1Group)
	globalHandler.registerPluginRoutes(v1Group)

	promHandler := promhttp.HandlerFor(pkgmetrics.DefaultGatherer(), promhttp.HandlerOpts{})
	router.GET("/metrics", func(ctx *gin.Context) {
		promHandler.ServeHTTP(ctx.Writer, ctx.Request)
	})

	router.GET(URLPathSwagger, ginswagger.WrapHandler(swaggerfiles.Handler))
	router.GET(URLPathHealthz, healthz())
	router.GET(URLPathMachineInfo, globalHandler.machineInfo)
	router.POST(URLPathInjectFault, globalHandler.injectFault)

	adminGroup := router.Group(urlPathAdmin)
	adminGroup.GET(urlPathConfig, handleAdminConfig(config))
	adminGroup.GET(urlPathPackages, handleAdminPackagesStatus(packageManager))

	if config.Pprof {
		log.Logger.Debugw("registering pprof handlers")
		adminGroup.GET("/pprof/profile", gin.WrapH(http.HandlerFunc(pprof.Profile)))
		adminGroup.GET("/pprof/heap", gin.WrapH(pprof.Handler("heap")))
		adminGroup.GET("/pprof/trace", gin.WrapH(http.HandlerFunc(pprof.Trace)))
	}

	epControlPlane, err := pkgmetadata.ReadMetadata(ctx, dbRO, pkgmetadata.MetadataKeyEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to read endpoint: %w", err)
	}
	s.epControlPlane = createURL(epControlPlane)

	s.epLocalGPUdServer, err = httputil.CreateURL("https", config.Address, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create local GPUd server endpoint: %w", err)
	}

	userToken := &UserToken{}
	go s.updateToken(ctx, metricsSQLiteStore, userToken)
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

func (s *Server) WaitUntilMachineID(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	log.Logger.Infow("waiting for machine id")
	for {
		s.machineIDMu.RLock()
		machineID := s.machineID
		s.machineIDMu.RUnlock()

		log.Logger.Infow("current server machine id", "id", machineID)

		if machineID != "" {
			break
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		machineID, err := pkgmetadata.ReadMachineIDWithFallback(ctx, s.dbRW, s.dbRO)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Logger.Errorw("failed to read machine uid", "error", err)
			continue
		}
		if machineID == "" {
			log.Logger.Debugw("machine id is not set, waiting for it to be set")
			continue
		}

		s.machineIDMu.Lock()
		s.machineID = machineID
		s.machineIDMu.Unlock()

		log.Logger.Infow("loaded server machine id", "id", machineID)
		break
	}
}

func (s *Server) updateToken(ctx context.Context, metricsStore pkgmetrics.Store, token *UserToken) {
	s.machineIDMu.RLock()
	machineID := s.machineID
	s.machineIDMu.RUnlock()

	var userToken string
	pipePath := s.fifoPath
	if dbToken, err := pkgmetadata.ReadTokenWithFallback(ctx, s.dbRW, s.dbRO, machineID); err == nil {
		userToken = dbToken

		token.mu.Lock()
		token.userToken = userToken
		token.mu.Unlock()
	}

	if userToken != "" {
		var err error
		s.session, err = session.NewSession(
			ctx,
			s.epLocalGPUdServer,
			s.epControlPlane,
			userToken,
			session.WithAuditLogger(s.auditLogger),
			session.WithMachineID(machineID),
			session.WithPipeInterval(3*time.Second),
			session.WithEnableAutoUpdate(s.enableAutoUpdate),
			session.WithAutoUpdateExitCode(s.autoUpdateExitCode),
			session.WithComponentsRegistry(s.componentsRegistry),
			session.WithNvidiaInstance(s.gpudInstance.NVMLInstance),
			session.WithMetricsStore(metricsStore),
			session.WithSavePluginSpecsFunc(func(ctx context.Context, specs pkgcustomplugins.Specs) (bool, error) {
				return pkgcustomplugins.SaveSpecs(s.pluginSpecsFile, specs)
			}),
			session.WithFaultInjector(s.faultInjector),
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
				s.epLocalGPUdServer,
				s.epControlPlane,
				userToken,
				session.WithAuditLogger(s.auditLogger),
				session.WithMachineID(machineID),
				session.WithPipeInterval(3*time.Second),
				session.WithEnableAutoUpdate(s.enableAutoUpdate),
				session.WithAutoUpdateExitCode(s.autoUpdateExitCode),
				session.WithComponentsRegistry(s.componentsRegistry),
				session.WithNvidiaInstance(s.gpudInstance.NVMLInstance),
				session.WithMetricsStore(metricsStore),
				session.WithSavePluginSpecsFunc(func(ctx context.Context, specs pkgcustomplugins.Specs) (bool, error) {
					return pkgcustomplugins.SaveSpecs(s.pluginSpecsFile, specs)
				}),
				session.WithFaultInjector(s.faultInjector),
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

		start := time.Now()
		err := sqlite.Compact(ctx, db)
		pkgmetricsrecorder.RecordSQLiteVacuum(time.Since(start).Seconds())

		if err != nil {
			log.Logger.Errorw("failed to compact state database", "error", err)
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
