package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/yaml"

	gpudconfig "github.com/leptonai/gpud/pkg/config"
)

const urlPathConfig = "/config"

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
