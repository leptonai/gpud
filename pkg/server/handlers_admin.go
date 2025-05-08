package server

import (
	"net/http"
	"path"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/yaml"

	gpudconfig "github.com/leptonai/gpud/pkg/config"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
)

const urlPathConfig = "/config"

func handleAdminConfig(cfg *gpudconfig.Config) func(c *gin.Context) {
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
	urlPathAdmin    = "/admin"
	urlPathPackages = "/packages"
)

var URLPathAdminPackages = path.Join(urlPathAdmin, urlPathPackages)

func handleAdminPackagesStatus(m *gpudmanager.Manager) func(c *gin.Context) {
	return func(c *gin.Context) {
		packageStatus, err := m.Status(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to get package status " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, packageStatus)
	}
}
