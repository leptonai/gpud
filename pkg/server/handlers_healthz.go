package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/yaml"
)

const URLPathHealthz = "/healthz"

type Healthz struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

var DefaultHealthz = Healthz{
	Status:  "ok",
	Version: "v1",
}

// healthz godoc
// @Summary Health check endpoint
// @Description Returns the health status of the gpud service
// @ID healthz
// @Tags health
// @Accept json
// @Produce json
// @Header 200 {string} Content-Type "application/json or application/yaml"
// @Success 200 {object} Healthz "Health status"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /healthz [get]
func healthz() func(ctx *gin.Context) {
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
