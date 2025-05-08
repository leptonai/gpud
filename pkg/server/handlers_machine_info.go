package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/leptonai/gpud/pkg/errdefs"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
)

const URLPathMachineInfo = "/machine-info"

func (g *globalHandler) handleMachineInfo(c *gin.Context) {
	if g.gpudInstance == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": "gpud instance not found"})
		return
	}

	info, err := pkgmachineinfo.GetMachineInfo(g.gpudInstance.NVMLInstance)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": errdefs.ErrUnknown, "message": "failed to get machine info: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, info)
}
