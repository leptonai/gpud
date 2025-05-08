package server

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/leptonai/gpud/components"
	gpudconfig "github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/errdefs"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const (
	URLPathSwagger     = "/swagger/*any"
	URLPathSwaggerDesc = "Swagger endpoint for docs"
)

const (
	RequestHeaderContentType = "Content-Type"
	RequestHeaderJSON        = "application/json"
	RequestHeaderYAML        = "application/yaml"
	RequestHeaderJSONIndent  = "json-indent"

	RequestHeaderAcceptEncoding = "Accept-Encoding"
	RequestHeaderEncodingGzip   = "gzip"
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
