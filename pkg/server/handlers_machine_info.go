package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/leptonai/gpud/pkg/errdefs"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
)

const URLPathMachineInfo = "/machine-info"

// machineInfo godoc
// @Summary Get machine information
// @Description Returns detailed information about the machine including hardware specifications
// @ID getMachineInfo
// @Tags machine
// @Produce json
// @Success 200 {object} pkgmachineinfo.MachineInfo "Machine information"
// @Failure 404 {object} map[string]interface{} "GPUd instance not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /machine-info [get]
func (g *globalHandler) machineInfo(c *gin.Context) {
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
