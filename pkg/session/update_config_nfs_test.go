package session

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	componentsnfs "github.com/leptonai/gpud/components/nfs"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
)

func TestProcessUpdateConfig_NFSSpecific(t *testing.T) {
	t.Run("valid nfs config passes validation", func(t *testing.T) {
		tempDir := t.TempDir()

		var capturedConfigs pkgnfschecker.Configs
		var wg sync.WaitGroup
		wg.Add(1)

		s := &Session{
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				capturedConfigs = cfgs
				wg.Done()
			},
		}

		config := pkgnfschecker.Configs{
			{
				VolumePath:   tempDir,
				DirName:      ".gpud-nfs-checker",
				FileContents: "test-content",
			},
		}

		configJSON, err := json.Marshal(config)
		assert.NoError(t, err)

		configMap := map[string]string{
			componentsnfs.Name: string(configJSON),
		}

		resp := &Response{}
		s.processUpdateConfig(configMap, resp)

		// Wait for async processing
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			// Processing completed
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for NFS config processing")
		}

		assert.Empty(t, resp.Error)
		assert.Equal(t, config, capturedConfigs)
	})

	t.Run("invalid nfs config still gets set", func(t *testing.T) {
		var capturedConfigs pkgnfschecker.Configs
		functionCalled := false
		var wg sync.WaitGroup
		wg.Add(1)

		s := &Session{
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				functionCalled = true
				capturedConfigs = cfgs
				wg.Done()
			},
		}

		// Invalid config - empty volume path
		config := pkgnfschecker.Configs{
			{
				VolumePath:   "",
				DirName:      ".gpud-nfs-checker",
				FileContents: "test-content",
			},
		}

		configJSON, err := json.Marshal(config)
		assert.NoError(t, err)

		configMap := map[string]string{
			componentsnfs.Name: string(configJSON),
		}

		resp := &Response{}
		s.processUpdateConfig(configMap, resp)

		// Wait for async processing
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			// Processing completed
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for NFS config processing")
		}

		// Error should be empty as validation failure is only logged
		assert.Empty(t, resp.Error)
		assert.True(t, functionCalled)
		assert.Equal(t, config, capturedConfigs)
	})

	t.Run("nfs config with nil function", func(t *testing.T) {
		tempDir := t.TempDir()

		s := &Session{
			setDefaultNFSGroupConfigsFunc: nil,
		}

		config := pkgnfschecker.Configs{
			{
				VolumePath:   tempDir,
				DirName:      ".gpud-nfs-checker",
				FileContents: "test-content",
			},
		}

		configJSON, err := json.Marshal(config)
		assert.NoError(t, err)

		configMap := map[string]string{
			componentsnfs.Name: string(configJSON),
		}

		resp := &Response{}

		// Start the process
		s.processUpdateConfig(configMap, resp)

		// Wait a bit for the async goroutine to complete validation
		// Since the function is nil, we just need to wait for validation to finish
		time.Sleep(500 * time.Millisecond)

		// Should not panic and no error
		assert.Empty(t, resp.Error)
	})

	t.Run("nfs config unmarshal error", func(t *testing.T) {
		s := &Session{
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				t.Error("should not be called on unmarshal error")
			},
		}

		configMap := map[string]string{
			componentsnfs.Name: `invalid json`,
		}

		resp := &Response{}
		s.processUpdateConfig(configMap, resp)

		assert.NotEmpty(t, resp.Error)
		assert.Contains(t, resp.Error, "invalid character")
	})

	t.Run("nfs config with validation timeout", func(t *testing.T) {
		tempDir := t.TempDir()

		validationCompleted := false
		var wg sync.WaitGroup
		wg.Add(1)

		s := &Session{
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				validationCompleted = true
				wg.Done()
			},
		}

		config := pkgnfschecker.Configs{
			{
				VolumePath:   tempDir,
				DirName:      ".gpud-nfs-checker",
				FileContents: "test-content",
			},
		}

		configJSON, err := json.Marshal(config)
		assert.NoError(t, err)

		configMap := map[string]string{
			componentsnfs.Name: string(configJSON),
		}

		resp := &Response{}
		s.processUpdateConfig(configMap, resp)

		// Wait for async processing
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			// Processing completed
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for NFS config processing")
		}

		assert.Empty(t, resp.Error)
		assert.True(t, validationCompleted)
	})
}

func TestProcessUpdateConfig_NFSJSONEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		configJSON     string
		expectError    bool
		errorContains  string
		expectFunction bool
	}{
		{
			name:           "empty array",
			configJSON:     `[]`,
			expectError:    false,
			expectFunction: true,
		},
		{
			name:           "null",
			configJSON:     `null`,
			expectError:    false,
			expectFunction: true,
		},
		{
			name:           "object instead of array",
			configJSON:     `{"volume_path": "/tmp"}`,
			expectError:    true,
			errorContains:  "cannot unmarshal object",
			expectFunction: false,
		},
		{
			name:           "string instead of array",
			configJSON:     `"not an array"`,
			expectError:    true,
			errorContains:  "cannot unmarshal string",
			expectFunction: false,
		},
		{
			name:           "number instead of array",
			configJSON:     `42`,
			expectError:    true,
			errorContains:  "cannot unmarshal number",
			expectFunction: false,
		},
		{
			name:           "boolean instead of array",
			configJSON:     `true`,
			expectError:    true,
			errorContains:  "cannot unmarshal bool",
			expectFunction: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			functionCalled := false
			var wg sync.WaitGroup
			if tt.expectFunction {
				wg.Add(1)
			}

			s := &Session{
				setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
					functionCalled = true
					if tt.expectFunction {
						wg.Done()
					}
				},
			}

			configMap := map[string]string{
				componentsnfs.Name: tt.configJSON,
			}

			resp := &Response{}
			s.processUpdateConfig(configMap, resp)

			if tt.expectFunction && !tt.expectError {
				// Wait for async processing
				done := make(chan struct{})
				go func() {
					wg.Wait()
					close(done)
				}()
				select {
				case <-done:
					// Processing completed
				case <-time.After(10 * time.Second):
					t.Fatal("Timeout waiting for NFS config processing")
				}
			}

			if tt.expectError {
				assert.NotEmpty(t, resp.Error)
				if tt.errorContains != "" {
					assert.Contains(t, resp.Error, tt.errorContains)
				}
			} else {
				assert.Empty(t, resp.Error)
			}

			assert.Equal(t, tt.expectFunction, functionCalled)
		})
	}
}
