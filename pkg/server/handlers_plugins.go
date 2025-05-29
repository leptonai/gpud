package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/yaml"

	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/httputil"
)

const URLPathComponentsCustomPlugins = "/plugins"

func (g *globalHandler) registerPluginRoutes(r gin.IRoutes) {
	r.GET(URLPathComponentsCustomPlugins, g.getPluginSpecs)
}

// getPluginSpecs godoc
// @Summary Get custom plugin specifications
// @Description Returns a list of all custom plugin specifications registered in the system
// @ID getPluginSpecs
// @Tags plugins
// @Accept json,yaml
// @Produce json,yaml
// @Header 200 {string} Content-Type "application/json or application/yaml"
// @Param json-indent header string false "Set to 'true' for indented JSON output"
// @Success 200 {array} pkgcustomplugins.Spec "List of custom plugin specifications"
// @Failure 400 {object} map[string]interface{} "Bad request - invalid content type"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /v1/plugins [get]
func (g *globalHandler) getPluginSpecs(c *gin.Context) {
	var specs pkgcustomplugins.Specs
	for _, c := range g.componentsRegistry.All() {
		if customPluginRegisteree, ok := c.(pkgcustomplugins.CustomPluginRegisteree); ok {
			if customPluginRegisteree.IsCustomPlugin() {
				specs = append(specs, customPluginRegisteree.Spec())
			}
		}
	}

	switch c.GetHeader(httputil.RequestHeaderContentType) {
	case httputil.RequestHeaderYAML:
		yb, err := yaml.Marshal(specs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal custom plugins " + err.Error()})
			return
		}
		c.String(http.StatusOK, string(yb))

	case httputil.RequestHeaderJSON, "":
		if c.GetHeader(httputil.RequestHeaderJSONIndent) == "true" {
			c.IndentedJSON(http.StatusOK, specs)
			return
		}
		c.JSON(http.StatusOK, specs)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid content type"})
	}
}
