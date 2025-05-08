package server

import (
	"net/http"
	"path"

	"github.com/gin-gonic/gin"

	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
)

const (
	urlPathAdmin    = "/admin"
	urlPathPackages = "/packages"
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
