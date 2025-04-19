package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/gin-gonic/gin"
	"github.com/leptonai/gpud/components"
	gpudconfig "github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/errdefs"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

type globalHandler struct {
	cfg *gpudconfig.Config

	componentsRegistry components.Registry

	componentNamesMu sync.RWMutex
	componentNames   []string

	metricsStore pkgmetrics.Store
}

func newGlobalHandler(cfg *gpudconfig.Config, componentsRegistry components.Registry, metricsStore pkgmetrics.Store) *globalHandler {
	var componentNames []string
	for _, c := range componentsRegistry.All() {
		componentNames = append(componentNames, c.Name())
	}
	sort.Strings(componentNames)

	return &globalHandler{
		cfg:                cfg,
		componentsRegistry: componentsRegistry,
		componentNames:     componentNames,
		metricsStore:       metricsStore,
	}
}

func (g *globalHandler) getReqTime(c *gin.Context) (time.Time, time.Time, error) {
	startTime := time.Now()
	endTime := time.Now()
	startTimeStr := c.Query("startTime")
	if startTimeStr != "" {
		startTimeInt, err := strconv.ParseInt(startTimeStr, 10, 64)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		startTime = time.Unix(startTimeInt, 0)
	}
	endTimeStr := c.Query("endTime")
	if endTimeStr != "" {
		endTimeInt, err := strconv.ParseInt(endTimeStr, 10, 64)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		endTime = time.Unix(endTimeInt, 0)
	}
	return startTime, endTime, nil
}

func (g *globalHandler) getReqComponents(c *gin.Context) ([]string, error) {
	components := c.Query("components")
	if components == "" {
		g.componentNamesMu.RLock()
		defer g.componentNamesMu.RUnlock()
		return g.componentNames, nil
	}

	var ret []string
	for _, component := range strings.Split(components, ",") {
		if c := g.componentsRegistry.Get(component); c == nil {
			return nil, fmt.Errorf("component %s not found (%w)", component, errdefs.ErrNotFound)
		}
		ret = append(ret, component)
	}
	return ret, nil
}

const (
	URLPathSwagger     = "/swagger/*any"
	URLPathSwaggerDesc = "Swagger endpoint for docs"
)

const (
	URLPathHealthz     = "/healthz"
	URLPathHealthzDesc = "Get the health status of the gpud instance"
)

type Healthz struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func (hz Healthz) JSON() ([]byte, error) {
	return json.Marshal(hz)
}

var DefaultHealthz = Healthz{
	Status:  "ok",
	Version: "v1",
}

func createHealthzHandler() func(ctx *gin.Context) {
	return func(c *gin.Context) {
		if c.GetHeader("Content-Type") == "application/yaml" {
			yb, err := yaml.Marshal(DefaultHealthz)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal components " + err.Error()})
				return
			}
			c.String(http.StatusOK, string(yb))
		} else {
			if c.GetHeader("json-indent") == "true" {
				c.IndentedJSON(http.StatusOK, DefaultHealthz)
			} else {
				c.JSON(http.StatusOK, DefaultHealthz)
			}
		}
	}
}

const (
	URLPathConfig     = "/config"
	URLPathConfigDesc = "Get the configuration of the gpud instance"
)

func createConfigHandler(cfg *gpudconfig.Config) func(c *gin.Context) {
	return func(c *gin.Context) {
		if c.GetHeader("Content-Type") == "application/yaml" {
			yb, err := yaml.Marshal(cfg)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal components " + err.Error()})
				return
			}
			c.String(http.StatusOK, string(yb))
		} else {
			if c.GetHeader("json-indent") == "true" {
				c.IndentedJSON(http.StatusOK, cfg)
			} else {
				c.JSON(http.StatusOK, cfg)
			}
		}
	}
}

const (
	urlPathAdmin        = "/admin"
	urlPathPackages     = "/packages"
	urlPathPackagesDesc = "Get the status of gpud managed packages"
)

var (
	URLPathAdminPackages = path.Join(urlPathAdmin, urlPathPackages)
)

func createPackageHandler(m *gpudmanager.Manager) func(c *gin.Context) {
	return func(c *gin.Context) {
		packageStatus, err := m.Status(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to get package status " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, packageStatus)
	}
}
