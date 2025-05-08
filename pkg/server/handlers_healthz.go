package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/yaml"
)

const urlPathHealthz = "/healthz"

type Healthz struct {
	Status  string `json:"status"`
	Version string `json:"version"`
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
