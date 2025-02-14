package gpudserver

import (
	"time"

	"github.com/gin-contrib/requestid"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// installRootGinMiddlewares installs gin middlewares for the root gin engine
func installRootGinMiddlewares(router *gin.Engine) {
	router.Use(requestid.New())
	router.ContextWithFallback = true
}

// installCommonGinMiddlewares installs common gin middlewares
func installCommonGinMiddlewares(router *gin.Engine, logger *zap.Logger) {
	// Add a ginzap middleware, which:
	//   - Logs all requests, like a combined access and error log.
	//   - Logs to stdout.
	//   - RFC3339 with UTC time format.
	router.Use(ginzap.Ginzap(logger, time.RFC3339, true))

	// Logs all panic to error log
	//   - stack means whether output the stack info.
	router.Use(ginzap.RecoveryWithZap(logger, true))
}
