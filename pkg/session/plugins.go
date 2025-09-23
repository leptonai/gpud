package session

import (
	"context"

	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/log"
)

func (s *Session) processSetPluginSpecs(ctx context.Context, resp *Response, specs pkgcustomplugins.Specs) (exitCode *int) {
	if s.savePluginSpecsFunc == nil {
		resp.Error = "save plugin specs function is not initialized"
		return
	}

	updated, err := s.savePluginSpecsFunc(ctx, specs)
	if err != nil {
		resp.Error = err.Error()
		return
	}
	log.Logger.Infow("successfully saved plugin specs", "plugins", len(specs))

	if updated {
		exitCode := 0
		log.Logger.Infow("scheduling auto exit for plugin specs update", "code", exitCode)
		return &exitCode
	}

	return nil
}

func (s *Session) processGetPluginSpecs(resp *Response) {
	specs := make(pkgcustomplugins.Specs, 0)
	for _, c := range s.componentsRegistry.All() {
		if registeree, ok := c.(pkgcustomplugins.CustomPluginRegisteree); ok && registeree.IsCustomPlugin() {
			specs = append(specs, registeree.Spec())
		}
	}
	resp.CustomPluginSpecs = specs
}
