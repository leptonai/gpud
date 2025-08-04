package session

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

func TestProcessUpdateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                  string
		configMap                             map[string]string
		setDefaultIbExpectedPortStatesFunc    func(states infiniband.ExpectedPortStates)
		setDefaultNFSGroupConfigsFunc         func(cfgs pkgnfschecker.Configs)
		setDefaultGPUCountsFunc               func(counts componentsnvidiagpucounts.ExpectedGPUCounts)
		expectedError                         string
		expectedIbExpectedPortStatesCalled    bool
		expectedNFSGroupConfigsCalled         bool
		expectedGPUCountsCalled               bool
		expectedIbExpectedPortStatesCallCount int
		expectedNFSGroupConfigsCallCount      int
		expectedGPUCountsCallCount            int
	}{
		{
			name:      "empty config map",
			configMap: map[string]string{},
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				t.Error("setDefaultIbExpectedPortStatesFunc should not be called for empty config map")
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				t.Error("setDefaultNFSGroupConfigsFunc should not be called for empty config map")
			},
			setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
				t.Error("setDefaultGPUCountsFunc should not be called for empty config map")
			},
			expectedError:                         "",
			expectedIbExpectedPortStatesCalled:    false,
			expectedNFSGroupConfigsCalled:         false,
			expectedGPUCountsCalled:               false,
			expectedIbExpectedPortStatesCallCount: 0,
			expectedNFSGroupConfigsCallCount:      0,
			expectedGPUCountsCallCount:            0,
		},
		{
			name: "valid infiniband config",
			configMap: map[string]string{
				"accelerator-nvidia-infiniband": `{"at_least_ports": 2, "at_least_rate": 100}`,
			},
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				assert.Equal(t, 2, states.AtLeastPorts)
				assert.Equal(t, 100, states.AtLeastRate)
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				// This gets called with empty config due to fallback behavior
				assert.Len(t, cfgs, 0)
			},
			setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
				// This gets called with empty config due to fallback behavior
				assert.Equal(t, 0, counts.Count)
			},
			expectedError:                         "",
			expectedIbExpectedPortStatesCalled:    true,
			expectedNFSGroupConfigsCalled:         true,
			expectedGPUCountsCalled:               true,
			expectedIbExpectedPortStatesCallCount: 1,
			expectedNFSGroupConfigsCallCount:      1,
			expectedGPUCountsCallCount:            1,
		},
		{
			name: "valid gpu counts config",
			configMap: map[string]string{
				"accelerator-nvidia-gpu-counts": `{"count": 8}`,
			},
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				// This gets called with empty config due to fallback behavior
				assert.Equal(t, 0, states.AtLeastPorts)
				assert.Equal(t, 0, states.AtLeastRate)
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				// This gets called with empty config due to fallback behavior
				assert.Len(t, cfgs, 0)
			},
			setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
				assert.Equal(t, 8, counts.Count)
			},
			expectedError:                         "",
			expectedIbExpectedPortStatesCalled:    true,
			expectedNFSGroupConfigsCalled:         true,
			expectedGPUCountsCalled:               true,
			expectedIbExpectedPortStatesCallCount: 1,
			expectedNFSGroupConfigsCallCount:      1,
			expectedGPUCountsCallCount:            1,
		},
		{
			name: "invalid infiniband config - malformed JSON",
			configMap: map[string]string{
				"accelerator-nvidia-infiniband": `{"at_least_ports": 2, "at_least_rate":}`,
			},
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				t.Error("setDefaultIbExpectedPortStatesFunc should not be called for invalid JSON")
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				t.Error("setDefaultNFSGroupConfigsFunc should not be called for infiniband config")
			},
			setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
				t.Error("setDefaultGPUCountsFunc should not be called for infiniband config")
			},
			expectedError:                         "invalid character '}' looking for beginning of value",
			expectedIbExpectedPortStatesCalled:    false,
			expectedNFSGroupConfigsCalled:         false,
			expectedGPUCountsCalled:               false,
			expectedIbExpectedPortStatesCallCount: 0,
			expectedNFSGroupConfigsCallCount:      0,
			expectedGPUCountsCallCount:            0,
		},
		{
			name: "invalid gpu counts config - malformed JSON",
			configMap: map[string]string{
				"accelerator-nvidia-gpu-counts": `{"count":}`,
			},
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				t.Error("setDefaultIbExpectedPortStatesFunc should not be called for gpu counts config")
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				t.Error("setDefaultNFSGroupConfigsFunc should not be called for gpu counts config")
			},
			setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
				t.Error("setDefaultGPUCountsFunc should not be called for invalid JSON")
			},
			expectedError:                         "invalid character '}' looking for beginning of value",
			expectedIbExpectedPortStatesCalled:    false,
			expectedNFSGroupConfigsCalled:         false,
			expectedGPUCountsCalled:               false,
			expectedIbExpectedPortStatesCallCount: 0,
			expectedNFSGroupConfigsCallCount:      0,
			expectedGPUCountsCallCount:            0,
		},
		{
			name: "invalid nfs config - malformed JSON",
			configMap: map[string]string{
				"nfs": `[{"volume_path": "/tmp/test", "ttl_to_delete": "5m", "num_expected_files":}]`,
			},
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				t.Error("setDefaultIbExpectedPortStatesFunc should not be called for nfs config")
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				t.Error("setDefaultNFSGroupConfigsFunc should not be called for invalid JSON")
			},
			setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
				t.Error("setDefaultGPUCountsFunc should not be called for nfs config")
			},
			expectedError:                         "invalid character '}' looking for beginning of value",
			expectedIbExpectedPortStatesCalled:    false,
			expectedNFSGroupConfigsCalled:         false,
			expectedGPUCountsCalled:               false,
			expectedIbExpectedPortStatesCallCount: 0,
			expectedNFSGroupConfigsCallCount:      0,
			expectedGPUCountsCallCount:            0,
		},
		{
			name: "invalid nfs config - validation error",
			configMap: map[string]string{
				"nfs": `[{"volume_path": "", "file_contents": "test-content", "ttl_to_delete": "5m", "num_expected_files": 3}]`,
			},
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				// This gets called with empty config due to fallback behavior
				assert.Equal(t, 0, states.AtLeastPorts)
				assert.Equal(t, 0, states.AtLeastRate)
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				// This function should be called even for invalid configs to allow user to fix them
				assert.Len(t, cfgs, 1)
				assert.Equal(t, "", cfgs[0].VolumePath) // invalid empty path
			},
			setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
				// This gets called with empty config due to fallback behavior
				assert.Equal(t, 0, counts.Count)
			},
			expectedError:                         "", // validation errors are logged but not returned as errors
			expectedIbExpectedPortStatesCalled:    true,
			expectedNFSGroupConfigsCalled:         true,
			expectedGPUCountsCalled:               true,
			expectedIbExpectedPortStatesCallCount: 1,
			expectedNFSGroupConfigsCallCount:      1, // function should be called even for invalid configs
			expectedGPUCountsCallCount:            1,
		},
		{
			name: "unsupported component",
			configMap: map[string]string{
				"unsupported-component": `{"some": "config"}`,
			},
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				// This gets called with empty config due to fallback behavior for unsupported components
				assert.Equal(t, 0, states.AtLeastPorts)
				assert.Equal(t, 0, states.AtLeastRate)
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				// This gets called with empty config due to fallback behavior for unsupported components
				assert.Len(t, cfgs, 0)
			},
			setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
				// This gets called with empty config due to fallback behavior for unsupported components
				assert.Equal(t, 0, counts.Count)
			},
			expectedError:                         "",
			expectedIbExpectedPortStatesCalled:    true,
			expectedNFSGroupConfigsCalled:         true,
			expectedGPUCountsCalled:               true,
			expectedIbExpectedPortStatesCallCount: 1,
			expectedNFSGroupConfigsCallCount:      1,
			expectedGPUCountsCallCount:            1,
		},
		{
			name: "nil function handlers",
			configMap: map[string]string{
				"accelerator-nvidia-infiniband": `{"at_least_ports": 2, "at_least_rate": 100}`,
			},
			setDefaultIbExpectedPortStatesFunc:    nil,
			setDefaultNFSGroupConfigsFunc:         nil,
			setDefaultGPUCountsFunc:               nil,
			expectedError:                         "",
			expectedIbExpectedPortStatesCalled:    false,
			expectedNFSGroupConfigsCalled:         false,
			expectedGPUCountsCalled:               false,
			expectedIbExpectedPortStatesCallCount: 0,
			expectedNFSGroupConfigsCallCount:      0,
			expectedGPUCountsCallCount:            0,
		},
		{
			name: "nil gpu counts function handler with gpu counts config",
			configMap: map[string]string{
				"accelerator-nvidia-gpu-counts": `{"count": 8}`,
			},
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				// This gets called with empty config due to fallback behavior
				assert.Equal(t, 0, states.AtLeastPorts)
				assert.Equal(t, 0, states.AtLeastRate)
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				// This gets called with empty config due to fallback behavior
				assert.Len(t, cfgs, 0)
			},
			setDefaultGPUCountsFunc:               nil,
			expectedError:                         "",
			expectedIbExpectedPortStatesCalled:    true,
			expectedNFSGroupConfigsCalled:         true,
			expectedGPUCountsCalled:               false,
			expectedIbExpectedPortStatesCallCount: 1,
			expectedNFSGroupConfigsCallCount:      1,
			expectedGPUCountsCallCount:            0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ibCallCount := 0
			nfsCallCount := 0
			gpuCallCount := 0

			// Add wait group for async NFS processing
			var wg sync.WaitGroup
			hasNFSConfig := false
			for componentName := range tt.configMap {
				if componentName == "nfs" {
					hasNFSConfig = true
					wg.Add(1)
					break
				}
			}

			// Create session with mock functions
			s := &Session{
				setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
					ibCallCount++
					if tt.setDefaultIbExpectedPortStatesFunc != nil {
						tt.setDefaultIbExpectedPortStatesFunc(states)
					}
				},
				setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
					nfsCallCount++
					if tt.setDefaultNFSGroupConfigsFunc != nil {
						tt.setDefaultNFSGroupConfigsFunc(cfgs)
					}
					if hasNFSConfig {
						wg.Done()
					}
				},
				setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
					gpuCallCount++
					if tt.setDefaultGPUCountsFunc != nil {
						tt.setDefaultGPUCountsFunc(counts)
					}
				},
			}

			// Handle nil function cases
			if tt.setDefaultIbExpectedPortStatesFunc == nil {
				s.setDefaultIbExpectedPortStatesFunc = nil
			}
			if tt.setDefaultNFSGroupConfigsFunc == nil {
				s.setDefaultNFSGroupConfigsFunc = nil
			}
			if tt.setDefaultGPUCountsFunc == nil {
				s.setDefaultGPUCountsFunc = nil
			}

			resp := &Response{}

			// Call the method under test
			s.processUpdateConfig(tt.configMap, resp)

			// Wait for async NFS processing to complete
			if hasNFSConfig && s.setDefaultNFSGroupConfigsFunc != nil && tt.expectedError == "" {
				done := make(chan struct{})
				go func() {
					wg.Wait()
					close(done)
				}()
				select {
				case <-done:
					// NFS processing completed
				case <-time.After(10 * time.Second):
					t.Fatal("Timeout waiting for NFS config processing")
				}
			}

			// Verify error
			if tt.expectedError != "" {
				assert.Contains(t, resp.Error, tt.expectedError)
			} else {
				assert.Empty(t, resp.Error)
			}

			// Verify function call counts
			assert.Equal(t, tt.expectedIbExpectedPortStatesCallCount, ibCallCount, "Unexpected infiniband function call count")
			assert.Equal(t, tt.expectedNFSGroupConfigsCallCount, nfsCallCount, "Unexpected NFS function call count")
			assert.Equal(t, tt.expectedGPUCountsCallCount, gpuCallCount, "Unexpected GPU counts function call count")
		})
	}

	// Test cases that need real directories
	t.Run("valid nfs config", func(t *testing.T) {
		tempDir := t.TempDir()

		ibCallCount := 0
		nfsCallCount := 0
		gpuCallCount := 0

		var wg sync.WaitGroup
		wg.Add(1)

		s := &Session{
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				ibCallCount++
				// This gets called with empty config due to fallback behavior
				assert.Equal(t, 0, states.AtLeastPorts)
				assert.Equal(t, 0, states.AtLeastRate)
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				nfsCallCount++
				assert.Len(t, cfgs, 1)
				assert.Equal(t, tempDir, cfgs[0].VolumePath)
				assert.Equal(t, "test-content", cfgs[0].FileContents)
				wg.Done()
			},
			setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
				gpuCallCount++
				// This gets called with empty config due to fallback behavior
				assert.Equal(t, 0, counts.Count)
			},
		}

		configMap := map[string]string{
			"nfs": `[{"volume_path": "` + tempDir + `", "file_contents": "test-content", "ttl_to_delete": "5m", "num_expected_files": 3}]`,
		}

		resp := &Response{}
		s.processUpdateConfig(configMap, resp)

		// Wait for async NFS processing
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			// NFS processing completed
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for NFS config processing")
		}

		assert.Empty(t, resp.Error)
		assert.Equal(t, 1, ibCallCount, "Unexpected infiniband function call count")
		assert.Equal(t, 1, nfsCallCount, "Unexpected NFS function call count")
		assert.Equal(t, 1, gpuCallCount, "Unexpected GPU counts function call count")
	})

	t.Run("multiple valid configs", func(t *testing.T) {
		tempDir := t.TempDir()

		ibCallCount := 0
		nfsCallCount := 0
		gpuCallCount := 0

		var wg sync.WaitGroup
		wg.Add(1)

		s := &Session{
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				ibCallCount++
				assert.Equal(t, 4, states.AtLeastPorts)
				assert.Equal(t, 200, states.AtLeastRate)
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				nfsCallCount++
				assert.Len(t, cfgs, 1)
				assert.Equal(t, tempDir, cfgs[0].VolumePath)
				assert.Equal(t, "multi-content", cfgs[0].FileContents)
				wg.Done()
			},
			setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
				gpuCallCount++
				assert.Equal(t, 16, counts.Count)
			},
		}

		configMap := map[string]string{
			"accelerator-nvidia-infiniband": `{"at_least_ports": 4, "at_least_rate": 200}`,
			"nfs":                           `[{"volume_path": "` + tempDir + `", "file_contents": "multi-content", "ttl_to_delete": "10m", "num_expected_files": 5}]`,
			"accelerator-nvidia-gpu-counts": `{"count": 16}`,
		}

		resp := &Response{}
		s.processUpdateConfig(configMap, resp)

		// Wait for async NFS processing
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			// NFS processing completed
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for NFS config processing")
		}

		assert.Empty(t, resp.Error)
		assert.Equal(t, 1, ibCallCount, "Unexpected infiniband function call count")
		assert.Equal(t, 1, nfsCallCount, "Unexpected NFS function call count")
		assert.Equal(t, 1, gpuCallCount, "Unexpected GPU counts function call count")
	})
}

func TestProcessUpdateConfig_JSONUnmarshalEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		componentName string
		configValue   string
		expectedError string
	}{
		{
			name:          "infiniband - empty JSON",
			componentName: "accelerator-nvidia-infiniband",
			configValue:   `{}`,
			expectedError: "",
		},
		{
			name:          "infiniband - null JSON",
			componentName: "accelerator-nvidia-infiniband",
			configValue:   `null`,
			expectedError: "",
		},
		{
			name:          "gpu counts - empty JSON",
			componentName: "accelerator-nvidia-gpu-counts",
			configValue:   `{}`,
			expectedError: "",
		},
		{
			name:          "gpu counts - null JSON",
			componentName: "accelerator-nvidia-gpu-counts",
			configValue:   `null`,
			expectedError: "",
		},
		{
			name:          "nfs - empty JSON array",
			componentName: "nfs",
			configValue:   `[]`,
			expectedError: "",
		},
		{
			name:          "nfs - null JSON",
			componentName: "nfs",
			configValue:   `null`,
			expectedError: "",
		},
		{
			name:          "nfs - empty object in array with validation error",
			componentName: "nfs",
			configValue:   `[{}]`,
			expectedError: "", // validation errors are logged but not returned as errors
		},
		{
			name:          "infiniband - invalid field type",
			componentName: "accelerator-nvidia-infiniband",
			configValue:   `{"at_least_ports": "invalid"}`,
			expectedError: "cannot unmarshal string into Go struct field",
		},
		{
			name:          "gpu counts - invalid field type",
			componentName: "accelerator-nvidia-gpu-counts",
			configValue:   `{"count": "invalid"}`,
			expectedError: "cannot unmarshal string into Go struct field",
		},
		{
			name:          "nfs - invalid field type",
			componentName: "nfs",
			configValue:   `[{"num_expected_files": "invalid"}]`,
			expectedError: "", // validation errors are logged but not returned as errors
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{
				setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {},
				setDefaultNFSGroupConfigsFunc:      func(cfgs pkgnfschecker.Configs) {},
				setDefaultGPUCountsFunc:            func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {},
			}

			configMap := map[string]string{
				tt.componentName: tt.configValue,
			}

			resp := &Response{}
			s.processUpdateConfig(configMap, resp)

			if tt.expectedError != "" {
				assert.Contains(t, resp.Error, tt.expectedError)
			} else {
				assert.Empty(t, resp.Error)
			}
		})
	}
}

func TestProcessUpdateConfig_RealConfigStructures(t *testing.T) {
	t.Parallel()

	t.Run("gpu counts with real structure", func(t *testing.T) {
		// Create a real componentsnvidiagpucounts.ExpectedGPUCounts structure
		expectedCounts := componentsnvidiagpucounts.ExpectedGPUCounts{
			Count: 4,
		}

		// Marshal it to JSON
		configBytes, err := json.Marshal(expectedCounts)
		assert.NoError(t, err)

		var actualCounts componentsnvidiagpucounts.ExpectedGPUCounts
		s := &Session{
			setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
				actualCounts = counts
			},
		}

		configMap := map[string]string{
			"accelerator-nvidia-gpu-counts": string(configBytes),
		}

		resp := &Response{}
		s.processUpdateConfig(configMap, resp)

		assert.Empty(t, resp.Error)
		assert.Equal(t, expectedCounts.Count, actualCounts.Count)
	})

	t.Run("infiniband with real structure", func(t *testing.T) {
		// Create a real infiniband.ExpectedPortStates structure
		expectedStates := infiniband.ExpectedPortStates{
			AtLeastPorts: 8,
			AtLeastRate:  400,
		}

		// Marshal it to JSON
		configBytes, err := json.Marshal(expectedStates)
		assert.NoError(t, err)

		var actualStates infiniband.ExpectedPortStates
		s := &Session{
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				actualStates = states
			},
		}

		configMap := map[string]string{
			"accelerator-nvidia-infiniband": string(configBytes),
		}

		resp := &Response{}
		s.processUpdateConfig(configMap, resp)

		assert.Empty(t, resp.Error)
		assert.Equal(t, expectedStates.AtLeastPorts, actualStates.AtLeastPorts)
		assert.Equal(t, expectedStates.AtLeastRate, actualStates.AtLeastRate)
	})

	t.Run("nfs with real structure", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create a real pkgnfschecker.GroupConfigs structure (slice)
		expectedConfigs := pkgnfschecker.Configs{
			{
				VolumePath:   tempDir,
				FileContents: "test-content",
			},
		}

		// Marshal it to JSON
		configBytes, err := json.Marshal(expectedConfigs)
		assert.NoError(t, err)

		var actualConfigs pkgnfschecker.Configs
		var wg sync.WaitGroup
		wg.Add(1)

		s := &Session{
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				actualConfigs = cfgs
				wg.Done()
			},
		}

		configMap := map[string]string{
			"nfs": string(configBytes),
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
		assert.Len(t, actualConfigs, 1)
		assert.Equal(t, expectedConfigs[0].VolumePath, actualConfigs[0].VolumePath)
		assert.Equal(t, expectedConfigs[0].FileContents, actualConfigs[0].FileContents)
	})

	t.Run("nfs with multiple configs", func(t *testing.T) {
		tempDir1 := t.TempDir()
		tempDir2 := t.TempDir()

		// Create multiple GroupConfig objects
		expectedConfigs := pkgnfschecker.Configs{
			{
				VolumePath:   tempDir1,
				FileContents: "test-content1",
			},
			{
				VolumePath:   tempDir2,
				FileContents: "test-content2",
			},
		}

		// Marshal it to JSON
		configBytes, err := json.Marshal(expectedConfigs)
		assert.NoError(t, err)

		var actualConfigs pkgnfschecker.Configs
		var wg sync.WaitGroup
		wg.Add(1)

		s := &Session{
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				actualConfigs = cfgs
				wg.Done()
			},
		}

		configMap := map[string]string{
			"nfs": string(configBytes),
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
		assert.Len(t, actualConfigs, 2)

		// Check first config
		assert.Equal(t, expectedConfigs[0].VolumePath, actualConfigs[0].VolumePath)
		assert.Equal(t, expectedConfigs[0].FileContents, actualConfigs[0].FileContents)

		// Check second config
		assert.Equal(t, expectedConfigs[1].VolumePath, actualConfigs[1].VolumePath)
		assert.Equal(t, expectedConfigs[1].FileContents, actualConfigs[1].FileContents)
	})
}
