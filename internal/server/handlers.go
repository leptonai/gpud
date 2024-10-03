package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	lep_components "github.com/leptonai/gpud/components"
	lep_config "github.com/leptonai/gpud/config"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/yaml"
)

type globalHandler struct {
	cfg        *lep_config.Config
	components map[string]lep_components.Component

	componentNamesMu sync.RWMutex
	componentNames   []string
}

func newGlobalHandler(cfg *lep_config.Config, components map[string]lep_components.Component) *globalHandler {
	var componentNames []string
	for name := range components {
		componentNames = append(componentNames, name)
	}
	sort.Strings(componentNames)

	return &globalHandler{
		cfg:            cfg,
		components:     components,
		componentNames: componentNames,
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
		if _, err := lep_components.GetComponent(component); err != nil {
			return nil, fmt.Errorf("failed to get component: %v", err)
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

func createConfigHandler(cfg *lep_config.Config) func(c *gin.Context) {
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
