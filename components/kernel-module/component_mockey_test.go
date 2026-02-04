package kernelmodule

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
)

// TestGetAllModules_ReadFileError tests getAllModules when os.ReadFile returns an error.
func TestGetAllModules_ReadFileError(t *testing.T) {
	mockey.PatchConvey("ReadFile error in getAllModules", t, func() {
		mockey.Mock(os.ReadFile).To(func(name string) ([]byte, error) {
			return nil, errors.New("permission denied")
		}).Build()

		modules, err := getAllModules()

		require.Error(t, err)
		assert.Nil(t, modules)
		assert.Contains(t, err.Error(), "failed to read")
		assert.Contains(t, err.Error(), "permission denied")
	})
}

// TestGetAllModules_ParseError tests getAllModules when parseEtcModules returns an error.
func TestGetAllModules_ParseError(t *testing.T) {
	mockey.PatchConvey("parseEtcModules error in getAllModules", t, func() {
		// Mock os.ReadFile to return valid data
		mockey.Mock(os.ReadFile).To(func(name string) ([]byte, error) {
			return []byte("module1\nmodule2"), nil
		}).Build()

		// Mock parseEtcModules to return an error
		mockey.Mock(parseEtcModules).To(func(b []byte) ([]string, error) {
			return nil, errors.New("parse error")
		}).Build()

		modules, err := getAllModules()

		require.Error(t, err)
		assert.Nil(t, modules)
		assert.Contains(t, err.Error(), "failed to parse")
	})
}

// TestGetAllModules_Success tests getAllModules with successful read and parse.
func TestGetAllModules_Success(t *testing.T) {
	mockey.PatchConvey("successful getAllModules", t, func() {
		mockey.Mock(os.ReadFile).To(func(name string) ([]byte, error) {
			return []byte("# comment\nmodule_b\nmodule_a\n"), nil
		}).Build()

		modules, err := getAllModules()

		require.NoError(t, err)
		assert.Len(t, modules, 2)
		// Modules should be sorted
		assert.Equal(t, "module_a", modules[0])
		assert.Equal(t, "module_b", modules[1])
	})
}

// TestCheckResultString_YamlMarshalError tests checkResult.String when yaml.Marshal fails.
func TestCheckResultString_YamlMarshalError(t *testing.T) {
	mockey.PatchConvey("yaml.Marshal error in checkResult.String", t, func() {
		mockey.Mock(yaml.Marshal).To(func(o interface{}) ([]byte, error) {
			return nil, errors.New("yaml marshal error")
		}).Build()

		cr := &checkResult{
			LoadedModules: []string{"module1", "module2"},
			health:        apiv1.HealthStateTypeHealthy,
			reason:        "all modules loaded",
		}

		result := cr.String()

		assert.Contains(t, result, "error marshaling data")
		assert.Contains(t, result, "yaml marshal error")
	})
}

// TestCheckResultString_NilWithMockey tests checkResult.String when checkResult is nil.
func TestCheckResultString_NilWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.String with nil checkResult", t, func() {
		var cr *checkResult
		result := cr.String()
		assert.Equal(t, "", result)
	})
}

// TestCheckResultSummary_NilWithMockey tests checkResult.Summary when checkResult is nil.
func TestCheckResultSummary_NilWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.Summary with nil checkResult", t, func() {
		var cr *checkResult
		result := cr.Summary()
		assert.Equal(t, "", result)
	})
}

// TestCheckResultHealthStateType_NilWithMockey tests checkResult.HealthStateType when checkResult is nil.
func TestCheckResultHealthStateType_NilWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.HealthStateType with nil checkResult", t, func() {
		var cr *checkResult
		result := cr.HealthStateType()
		assert.Equal(t, apiv1.HealthStateType(""), result)
	})
}

// TestCheckResultGetError_NilWithMockey tests checkResult.getError when checkResult is nil.
func TestCheckResultGetError_NilWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.getError with nil checkResult", t, func() {
		var cr *checkResult
		result := cr.getError()
		assert.Equal(t, "", result)
	})
}

// TestCheckResultGetError_NilErrorWithMockey tests checkResult.getError when err is nil.
func TestCheckResultGetError_NilErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.getError with nil err", t, func() {
		cr := &checkResult{
			health: apiv1.HealthStateTypeHealthy,
			reason: "all good",
		}
		result := cr.getError()
		assert.Equal(t, "", result)
	})
}

// TestCheckResultGetError_WithErrorWithMockey tests checkResult.getError when err is set.
func TestCheckResultGetError_WithErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.getError with err set", t, func() {
		cr := &checkResult{
			err:    errors.New("test error"),
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "error occurred",
		}
		result := cr.getError()
		assert.Equal(t, "test error", result)
	})
}

// TestCheckResultHealthStates_NilWithMockey tests checkResult.HealthStates when checkResult is nil.
func TestCheckResultHealthStates_NilWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.HealthStates with nil checkResult", t, func() {
		var cr *checkResult
		states := cr.HealthStates()

		require.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
	})
}

// TestCheckResultHealthStates_WithErrorWithMockey tests checkResult.HealthStates with an error set.
func TestCheckResultHealthStates_WithErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.HealthStates with error set", t, func() {
		cr := &checkResult{
			ts:     time.Now().UTC(),
			err:    errors.New("module load failed"),
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "error getting all modules",
		}
		states := cr.HealthStates()

		require.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "error getting all modules", states[0].Reason)
		assert.Equal(t, "module load failed", states[0].Error)
	})
}

// TestCheckResultHealthStates_WithLoadedModulesWithMockey tests checkResult.HealthStates with loaded modules.
func TestCheckResultHealthStates_WithLoadedModulesWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.HealthStates with loaded modules", t, func() {
		cr := &checkResult{
			LoadedModules: []string{"module1", "module2"},
			loadedModules: map[string]struct{}{"module1": {}, "module2": {}},
			ts:            time.Now().UTC(),
			health:        apiv1.HealthStateTypeHealthy,
			reason:        "all modules are loaded",
		}
		states := cr.HealthStates()

		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Contains(t, states[0].ExtraInfo, "data")
		assert.Contains(t, states[0].ExtraInfo["data"], "module1")
		assert.Contains(t, states[0].ExtraInfo["data"], "module2")
	})
}

// TestCheckResultHealthStates_EmptyLoadedModulesWithMockey tests checkResult.HealthStates with empty loaded modules.
func TestCheckResultHealthStates_EmptyLoadedModulesWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.HealthStates with empty loaded modules", t, func() {
		cr := &checkResult{
			LoadedModules: []string{},
			ts:            time.Now().UTC(),
			health:        apiv1.HealthStateTypeHealthy,
			reason:        "all modules are loaded",
		}
		states := cr.HealthStates()

		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		// ExtraInfo should be nil or empty when LoadedModules is empty
		assert.Empty(t, states[0].ExtraInfo)
	})
}

// TestComponentNew_WithMockey tests creating a new component using mockey.
func TestComponentNew_WithMockey(t *testing.T) {
	mockey.PatchConvey("New component creation", t, func() {
		gpudInstance := &components.GPUdInstance{
			KernelModulesToCheck: []string{"nvidia", "nvidia_uvm"},
		}

		c, err := New(gpudInstance)

		require.NoError(t, err)
		require.NotNil(t, c)
		assert.Equal(t, Name, c.Name())

		comp, ok := c.(*component)
		require.True(t, ok)
		assert.NotNil(t, comp.ctx)
		assert.NotNil(t, comp.cancel)
		assert.NotNil(t, comp.getAllModulesFunc)
		assert.Equal(t, []string{"nvidia", "nvidia_uvm"}, comp.modulesToCheck)
	})
}

// TestComponentIsSupportedWithMockey tests the IsSupported method.
func TestComponentIsSupportedWithMockey(t *testing.T) {
	mockey.PatchConvey("IsSupported always returns true", t, func() {
		c, err := New(&components.GPUdInstance{})
		require.NoError(t, err)

		result := c.(*component).IsSupported()
		assert.True(t, result)
	})
}

// TestComponentStart_WithMockey tests the Start method.
func TestComponentStart_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start begins background check loop", t, func() {
		gpudInstance := &components.GPUdInstance{}
		c, err := New(gpudInstance)
		require.NoError(t, err)
		comp := c.(*component)

		// Mock getAllModulesFunc to prevent actual file access
		comp.getAllModulesFunc = func() ([]string, error) {
			return []string{"module1"}, nil
		}

		err = comp.Start()
		require.NoError(t, err)

		// Give the goroutine a moment to execute
		time.Sleep(50 * time.Millisecond)

		// Close should cancel the context and stop the loop
		err = comp.Close()
		require.NoError(t, err)
	})
}

// TestComponentClose_WithMockey tests the Close method.
func TestComponentClose_WithMockey(t *testing.T) {
	mockey.PatchConvey("Close cancels context", t, func() {
		c, err := New(&components.GPUdInstance{})
		require.NoError(t, err)
		comp := c.(*component)

		err = comp.Close()
		require.NoError(t, err)

		// Verify context is canceled
		select {
		case <-comp.ctx.Done():
			// Expected - context should be canceled
		default:
			t.Error("expected context to be canceled after Close")
		}
	})
}

// TestComponentCheck_GetAllModulesError tests Check when getAllModulesFunc returns an error.
func TestComponentCheck_GetAllModulesError(t *testing.T) {
	mockey.PatchConvey("Check with getAllModules error", t, func() {
		c, err := New(&components.GPUdInstance{
			KernelModulesToCheck: []string{"nvidia"},
		})
		require.NoError(t, err)
		comp := c.(*component)

		testErr := errors.New("failed to read modules")
		comp.getAllModulesFunc = func() ([]string, error) {
			return nil, testErr
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "error getting all modules", cr.reason)
		assert.Equal(t, testErr, cr.err)
	})
}

// TestComponentCheck_AllModulesLoaded tests Check when all required modules are loaded.
func TestComponentCheck_AllModulesLoaded(t *testing.T) {
	mockey.PatchConvey("Check with all modules loaded", t, func() {
		c, err := New(&components.GPUdInstance{
			KernelModulesToCheck: []string{"nvidia", "nvidia_uvm"},
		})
		require.NoError(t, err)
		comp := c.(*component)

		comp.getAllModulesFunc = func() ([]string, error) {
			return []string{"nvidia", "nvidia_uvm", "nvidia_drm"}, nil
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "all modules are loaded", cr.reason)
		assert.Nil(t, cr.err)
		assert.Len(t, cr.LoadedModules, 3)
	})
}

// TestComponentCheck_MissingModules tests Check when some required modules are missing.
func TestComponentCheck_MissingModules(t *testing.T) {
	mockey.PatchConvey("Check with missing modules", t, func() {
		c, err := New(&components.GPUdInstance{
			KernelModulesToCheck: []string{"nvidia", "nvidia_uvm", "nvidia_drm"},
		})
		require.NoError(t, err)
		comp := c.(*component)

		comp.getAllModulesFunc = func() ([]string, error) {
			return []string{"nvidia"}, nil
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "missing modules")
		assert.Contains(t, cr.reason, "nvidia_drm")
		assert.Contains(t, cr.reason, "nvidia_uvm")
		assert.Nil(t, cr.err)
	})
}

// TestComponentCheck_NoModulesToCheck tests Check when no modules are configured to check.
func TestComponentCheck_NoModulesToCheck(t *testing.T) {
	mockey.PatchConvey("Check with no modules to check", t, func() {
		c, err := New(&components.GPUdInstance{
			KernelModulesToCheck: nil,
		})
		require.NoError(t, err)
		comp := c.(*component)

		comp.getAllModulesFunc = func() ([]string, error) {
			return []string{"module1", "module2"}, nil
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "all modules are loaded", cr.reason)
	})
}

// TestComponentCheck_EmptyLoadedModules tests Check when no modules are loaded.
func TestComponentCheck_EmptyLoadedModules(t *testing.T) {
	mockey.PatchConvey("Check with empty loaded modules", t, func() {
		c, err := New(&components.GPUdInstance{
			KernelModulesToCheck: []string{"nvidia"},
		})
		require.NoError(t, err)
		comp := c.(*component)

		comp.getAllModulesFunc = func() ([]string, error) {
			return []string{}, nil
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "missing modules")
		assert.Contains(t, cr.reason, "nvidia")
	})
}

// TestComponentLastHealthStates_BeforeCheck tests LastHealthStates before any check has run.
func TestComponentLastHealthStates_BeforeCheck(t *testing.T) {
	mockey.PatchConvey("LastHealthStates before Check", t, func() {
		c, err := New(&components.GPUdInstance{})
		require.NoError(t, err)
		comp := c.(*component)

		states := comp.LastHealthStates()

		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
	})
}

// TestComponentLastHealthStates_AfterCheck tests LastHealthStates after a check has run.
func TestComponentLastHealthStates_AfterCheck(t *testing.T) {
	mockey.PatchConvey("LastHealthStates after Check", t, func() {
		c, err := New(&components.GPUdInstance{
			KernelModulesToCheck: []string{"nvidia"},
		})
		require.NoError(t, err)
		comp := c.(*component)

		comp.getAllModulesFunc = func() ([]string, error) {
			return []string{"nvidia"}, nil
		}

		comp.Check()

		states := comp.LastHealthStates()

		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "all modules are loaded", states[0].Reason)
	})
}

// TestComponentTagsWithMockey tests the Tags method.
func TestComponentTagsWithMockey(t *testing.T) {
	mockey.PatchConvey("Tags returns component name", t, func() {
		c, err := New(&components.GPUdInstance{})
		require.NoError(t, err)

		tags := c.(*component).Tags()

		assert.Equal(t, []string{Name}, tags)
	})
}

// TestCheckResultComponentNameWithMockey tests the ComponentName method.
func TestCheckResultComponentNameWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult ComponentName method", t, func() {
		cr := &checkResult{}
		assert.Equal(t, Name, cr.ComponentName())
	})
}

// TestCheckResultString_WithModulesWithMockey tests checkResult.String with loaded modules.
func TestCheckResultString_WithModulesWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.String with loaded modules", t, func() {
		cr := &checkResult{
			LoadedModules: []string{"module1", "module2"},
			health:        apiv1.HealthStateTypeHealthy,
			reason:        "all modules loaded",
		}

		result := cr.String()

		assert.Contains(t, result, "module1")
		assert.Contains(t, result, "module2")
	})
}

// TestParseEtcModulesWithMockey_EmptyFile tests parseEtcModules with an empty file.
func TestParseEtcModulesWithMockey_EmptyFile(t *testing.T) {
	mockey.PatchConvey("parseEtcModules with empty file", t, func() {
		modules, err := parseEtcModules([]byte(""))

		require.NoError(t, err)
		assert.Empty(t, modules)
	})
}

// TestParseEtcModulesWithMockey_OnlyComments tests parseEtcModules with only comments.
func TestParseEtcModulesWithMockey_OnlyComments(t *testing.T) {
	mockey.PatchConvey("parseEtcModules with only comments", t, func() {
		input := `# This is a comment
# Another comment
`
		modules, err := parseEtcModules([]byte(input))

		require.NoError(t, err)
		assert.Empty(t, modules)
	})
}

// TestParseEtcModulesWithMockey_OnlyWhitespace tests parseEtcModules with only whitespace.
func TestParseEtcModulesWithMockey_OnlyWhitespace(t *testing.T) {
	mockey.PatchConvey("parseEtcModules with only whitespace", t, func() {
		input := `


`
		modules, err := parseEtcModules([]byte(input))

		require.NoError(t, err)
		assert.Empty(t, modules)
	})
}

// TestParseEtcModulesWithMockey_MixedContent tests parseEtcModules with mixed content.
func TestParseEtcModulesWithMockey_MixedContent(t *testing.T) {
	mockey.PatchConvey("parseEtcModules with mixed content", t, func() {
		input := `# Header comment
module_c
# Middle comment
module_a
   module_b
`
		modules, err := parseEtcModules([]byte(input))

		require.NoError(t, err)
		require.Len(t, modules, 3)
		// Modules should be sorted
		assert.Equal(t, "module_a", modules[0])
		assert.Equal(t, "module_b", modules[1])
		assert.Equal(t, "module_c", modules[2])
	})
}

// TestParseEtcModulesWithMockey_WithOptions tests parseEtcModules with module options.
func TestParseEtcModulesWithMockey_WithOptions(t *testing.T) {
	mockey.PatchConvey("parseEtcModules with module options", t, func() {
		input := `module_name option=value`
		modules, err := parseEtcModules([]byte(input))

		require.NoError(t, err)
		require.Len(t, modules, 1)
		assert.Equal(t, "module_name option=value", modules[0])
	})
}

// TestComponentCheck_TimestampSet tests that Check sets the timestamp correctly.
func TestComponentCheck_TimestampSet(t *testing.T) {
	mockey.PatchConvey("Check sets timestamp", t, func() {
		c, err := New(&components.GPUdInstance{})
		require.NoError(t, err)
		comp := c.(*component)

		comp.getAllModulesFunc = func() ([]string, error) {
			return []string{}, nil
		}

		beforeCheck := time.Now().UTC()
		time.Sleep(1 * time.Millisecond)
		comp.Check()
		time.Sleep(1 * time.Millisecond)
		afterCheck := time.Now().UTC()

		comp.lastMu.RLock()
		ts := comp.lastCheckResult.ts
		comp.lastMu.RUnlock()

		assert.True(t, !ts.Before(beforeCheck), "timestamp should be after check started")
		assert.True(t, !ts.After(afterCheck), "timestamp should be before check ended")
	})
}

// TestComponentCheck_UpdatesLastCheckResult tests that Check updates lastCheckResult.
func TestComponentCheck_UpdatesLastCheckResult(t *testing.T) {
	mockey.PatchConvey("Check updates lastCheckResult", t, func() {
		c, err := New(&components.GPUdInstance{
			KernelModulesToCheck: []string{"module1"},
		})
		require.NoError(t, err)
		comp := c.(*component)

		// First check - modules present
		comp.getAllModulesFunc = func() ([]string, error) {
			return []string{"module1"}, nil
		}
		comp.Check()

		comp.lastMu.RLock()
		firstHealth := comp.lastCheckResult.health
		comp.lastMu.RUnlock()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, firstHealth)

		// Second check - modules missing
		comp.getAllModulesFunc = func() ([]string, error) {
			return []string{}, nil
		}
		comp.Check()

		comp.lastMu.RLock()
		secondHealth := comp.lastCheckResult.health
		comp.lastMu.RUnlock()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, secondHealth)
	})
}

// TestComponentCheck_ConcurrentSafety tests concurrent access to Check and LastHealthStates.
func TestComponentCheck_ConcurrentSafety(t *testing.T) {
	mockey.PatchConvey("Concurrent Check and LastHealthStates", t, func() {
		c, err := New(&components.GPUdInstance{
			KernelModulesToCheck: []string{"module1"},
		})
		require.NoError(t, err)
		comp := c.(*component)

		comp.getAllModulesFunc = func() ([]string, error) {
			time.Sleep(1 * time.Millisecond) // Small delay to increase chance of race
			return []string{"module1"}, nil
		}

		done := make(chan bool, 20)

		// Run concurrent checks
		for i := 0; i < 10; i++ {
			go func() {
				comp.Check()
				done <- true
			}()
			go func() {
				_ = comp.LastHealthStates()
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 20; i++ {
			<-done
		}

		// Final state should be consistent
		states := comp.LastHealthStates()
		require.Len(t, states, 1)
	})
}
