package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestInstallRootGinMiddlewares(t *testing.T) {
	router := gin.New()
	installRootGinMiddlewares(router)

	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "test")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Header().Get("X-Request-Id"), "X-Request-Id header should be set")

	// Verify that ContextWithFallback was set
	assert.True(t, router.ContextWithFallback, "ContextWithFallback should be true")
}

func TestInstallCommonGinMiddlewares(t *testing.T) {
	// Create a test logger
	logger := zaptest.NewLogger(t)
	router := gin.New()
	installCommonGinMiddlewares(router, logger)

	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "test")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	router.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	req = httptest.NewRequest("GET", "/panic", nil)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Check that we got a 500 error but the server didn't crash
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
