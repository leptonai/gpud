package server

import (
	"fmt"
	"os"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/log"
)

func (s *Server) loadPluginSpecs(path string) error {
	if path != "" {
		_, err := os.Stat(path)
		exists := err == nil

		if exists {
			specs, err := pkgcustomplugins.LoadSpecs(path)
			if err != nil {
				return fmt.Errorf("failed to load plugin specs: %w", err)
			}

			for _, spec := range specs {
				initFunc := spec.NewInitFunc()
				if initFunc == nil {
					log.Logger.Errorw("failed to load plugin", "name", spec.ComponentName())
					continue
				}

				if spec.PluginType == pkgcustomplugins.SpecTypeInit {
					if _, err := s.initRegistry.Register(initFunc); err != nil {
						return fmt.Errorf("failed to register init plugin %s: %w", spec.ComponentName(), err)
					}
					log.Logger.Infow("loaded init plugin", "name", spec.ComponentName())
				} else {
					if _, err := s.componentsRegistry.Register(initFunc); err != nil {
						return fmt.Errorf("failed to register component plugin %s: %w", spec.ComponentName(), err)
					}
					log.Logger.Infow("loaded component plugin", "name", spec.ComponentName())
				}
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect plugin specs: %w", err)
		} else {
			log.Logger.Warnw("plugin specs file does not exist, skipping", "path", path)
		}
	}

	// init plugin run only "once", and "before" regular components
	// thus no need to start
	for _, c := range s.initRegistry.All() {
		rs := c.Check()
		if rs.HealthStateType() != apiv1.HealthStateTypeHealthy {
			return fmt.Errorf("failed to start init plugin %s: %s", c.Name(), rs.Summary())
		}
		log.Logger.Infow("successfully executed init plugin", "name", c.Name(), "summary", rs.Summary())

		debugger, ok := rs.(components.CheckResultDebugger)
		if ok {
			fmt.Printf("init plugin debug output %q:\n\n%s\n\n", c.Name(), debugger.Debug())
		}
	}

	return nil
}

// Keep the session available for a corrected profile; discard every plugin
// from this profile so a partial registration cannot silently remain active.
func (s *Server) quarantinePluginSpecs(path string, cause error) {
	for _, registry := range []components.Registry{s.initRegistry, s.componentsRegistry} {
		for _, c := range registry.All() {
			if plugin, ok := c.(pkgcustomplugins.CustomPluginRegisteree); ok && plugin.IsCustomPlugin() {
				registry.Deregister(c.Name())
				if err := c.Close(); err != nil {
					log.Logger.Warnw("failed to close quarantined plugin", "name", c.Name(), "error", err)
				}
			}
		}
	}
	if err := os.Rename(path, path+".invalid"); err != nil {
		log.Logger.Errorw("failed to quarantine plugin specs; continuing without plugins", "path", path, "error", err, "cause", cause)
		return
	}
	log.Logger.Errorw("quarantined plugin specs; continuing without plugins", "path", path, "quarantine", path+".invalid", "error", cause)
}

func (s *Server) savePluginSpecs(specs pkgcustomplugins.Specs) (bool, error) {
	// A valid but unhealthy init plugin can otherwise be saved and restarted
	// on every control-plane sync after quarantine. Require a changed profile.
	if s.pluginSpecsFileManaged && len(specs) > 0 {
		if rejected, err := pkgcustomplugins.LoadSpecs(s.pluginSpecsFile + ".invalid"); err == nil && rejected.Equal(specs) {
			return false, fmt.Errorf("plugin profile previously failed startup; send a corrected profile")
		}
	}
	return pkgcustomplugins.SaveSpecs(s.pluginSpecsFile, specs)
}
