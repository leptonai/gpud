package server

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/leptonai/gpud/pkg/errdefs"
	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
)

const URLPathInjectFault = "/inject-fault"

func (g *globalHandler) handleInjectFault(c *gin.Context) {
	if g.faultInjector == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": "fault injector not set up"})
		return
	}

	// read the request body
	request := new(pkgfaultinjector.Request)
	if err := json.NewDecoder(c.Request.Body).Decode(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to decode request body: " + err.Error()})
		return
	}
	if err := request.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid request: " + err.Error()})
		return
	}

	switch {
	case request.KernelMessage != nil:
		if err := g.faultInjector.InjectKernelMessage(c.Request.Context(), request.KernelMessage); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": errdefs.ErrUnknown, "message": "failed to inject kernel message: " + err.Error()})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "kernel message is required"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "fault injected"})
}
