package session

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
		expectedError                         string
		expectedIbExpectedPortStatesCalled    bool
		expectedNFSGroupConfigsCalled         bool
		expectedIbExpectedPortStatesCallCount int
		expectedNFSGroupConfigsCallCount      int
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
			expectedError:                         "",
			expectedIbExpectedPortStatesCalled:    false,
			expectedNFSGroupConfigsCalled:         false,
			expectedIbExpectedPortStatesCallCount: 0,
			expectedNFSGroupConfigsCallCount:      0,
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
				t.Error("setDefaultNFSGroupConfigsFunc should not be called for infiniband config")
			},
			expectedError:                         "",
			expectedIbExpectedPortStatesCalled:    true,
			expectedNFSGroupConfigsCalled:         false,
			expectedIbExpectedPortStatesCallCount: 1,
			expectedNFSGroupConfigsCallCount:      0,
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
			expectedError:                         "invalid character '}' looking for beginning of value",
			expectedIbExpectedPortStatesCalled:    false,
			expectedNFSGroupConfigsCalled:         false,
			expectedIbExpectedPortStatesCallCount: 0,
			expectedNFSGroupConfigsCallCount:      0,
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
			expectedError:                         "invalid character '}' looking for beginning of value",
			expectedIbExpectedPortStatesCalled:    false,
			expectedNFSGroupConfigsCalled:         false,
			expectedIbExpectedPortStatesCallCount: 0,
			expectedNFSGroupConfigsCallCount:      0,
		},
		{
			name: "invalid nfs config - validation error",
			configMap: map[string]string{
				"nfs": `[{"volume_path": "", "file_contents": "test-content", "ttl_to_delete": "5m", "num_expected_files": 3}]`,
			},
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				t.Error("setDefaultIbExpectedPortStatesFunc should not be called for nfs config")
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				t.Error("setDefaultNFSGroupConfigsFunc should not be called for invalid config")
			},
			expectedError:                         "volume path is empty",
			expectedIbExpectedPortStatesCalled:    false,
			expectedNFSGroupConfigsCalled:         false,
			expectedIbExpectedPortStatesCallCount: 0,
			expectedNFSGroupConfigsCallCount:      0,
		},
		{
			name: "unsupported component",
			configMap: map[string]string{
				"unsupported-component": `{"some": "config"}`,
			},
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				t.Error("setDefaultIbExpectedPortStatesFunc should not be called for unsupported component")
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				t.Error("setDefaultNFSGroupConfigsFunc should not be called for unsupported component")
			},
			expectedError:                         "",
			expectedIbExpectedPortStatesCalled:    false,
			expectedNFSGroupConfigsCalled:         false,
			expectedIbExpectedPortStatesCallCount: 0,
			expectedNFSGroupConfigsCallCount:      0,
		},
		{
			name: "nil function handlers",
			configMap: map[string]string{
				"accelerator-nvidia-infiniband": `{"at_least_ports": 2, "at_least_rate": 100}`,
			},
			setDefaultIbExpectedPortStatesFunc:    nil,
			setDefaultNFSGroupConfigsFunc:         nil,
			expectedError:                         "",
			expectedIbExpectedPortStatesCalled:    false,
			expectedNFSGroupConfigsCalled:         false,
			expectedIbExpectedPortStatesCallCount: 0,
			expectedNFSGroupConfigsCallCount:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ibCallCount := 0
			nfsCallCount := 0

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
				},
			}

			// Handle nil function cases
			if tt.setDefaultIbExpectedPortStatesFunc == nil {
				s.setDefaultIbExpectedPortStatesFunc = nil
			}
			if tt.setDefaultNFSGroupConfigsFunc == nil {
				s.setDefaultNFSGroupConfigsFunc = nil
			}

			resp := &Response{}

			// Call the method under test
			s.processUpdateConfig(tt.configMap, resp)

			// Verify error
			if tt.expectedError != "" {
				assert.Contains(t, resp.Error, tt.expectedError)
			} else {
				assert.Empty(t, resp.Error)
			}

			// Verify function call counts
			assert.Equal(t, tt.expectedIbExpectedPortStatesCallCount, ibCallCount, "Unexpected infiniband function call count")
			assert.Equal(t, tt.expectedNFSGroupConfigsCallCount, nfsCallCount, "Unexpected NFS function call count")
		})
	}

	// Test cases that need real directories
	t.Run("valid nfs config", func(t *testing.T) {
		tempDir := t.TempDir()

		ibCallCount := 0
		nfsCallCount := 0

		s := &Session{
			setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {
				ibCallCount++
				t.Error("setDefaultIbExpectedPortStatesFunc should not be called for nfs config")
			},
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				nfsCallCount++
				assert.Len(t, cfgs, 1)
				assert.Equal(t, tempDir, cfgs[0].VolumePath)
				assert.Equal(t, "test-content", cfgs[0].FileContents)
				assert.Equal(t, 5*time.Minute, cfgs[0].TTLToDelete.Duration)
				assert.Equal(t, 3, cfgs[0].NumExpectedFiles)
			},
		}

		configMap := map[string]string{
			"nfs": `[{"volume_path": "` + tempDir + `", "file_contents": "test-content", "ttl_to_delete": "5m", "num_expected_files": 3}]`,
		}

		resp := &Response{}
		s.processUpdateConfig(configMap, resp)

		assert.Empty(t, resp.Error)
		assert.Equal(t, 0, ibCallCount, "Unexpected infiniband function call count")
		assert.Equal(t, 1, nfsCallCount, "Unexpected NFS function call count")
	})

	t.Run("multiple valid configs", func(t *testing.T) {
		tempDir := t.TempDir()

		ibCallCount := 0
		nfsCallCount := 0

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
				assert.Equal(t, 10*time.Minute, cfgs[0].TTLToDelete.Duration)
				assert.Equal(t, 5, cfgs[0].NumExpectedFiles)
			},
		}

		configMap := map[string]string{
			"accelerator-nvidia-infiniband": `{"at_least_ports": 4, "at_least_rate": 200}`,
			"nfs":                           `[{"volume_path": "` + tempDir + `", "file_contents": "multi-content", "ttl_to_delete": "10m", "num_expected_files": 5}]`,
		}

		resp := &Response{}
		s.processUpdateConfig(configMap, resp)

		assert.Empty(t, resp.Error)
		assert.Equal(t, 1, ibCallCount, "Unexpected infiniband function call count")
		assert.Equal(t, 1, nfsCallCount, "Unexpected NFS function call count")
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
			expectedError: "volume path is empty",
		},
		{
			name:          "infiniband - invalid field type",
			componentName: "accelerator-nvidia-infiniband",
			configValue:   `{"at_least_ports": "invalid"}`,
			expectedError: "cannot unmarshal string into Go struct field",
		},
		{
			name:          "nfs - invalid field type",
			componentName: "nfs",
			configValue:   `[{"num_expected_files": "invalid"}]`,
			expectedError: "cannot unmarshal string into Go struct field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{
				setDefaultIbExpectedPortStatesFunc: func(states infiniband.ExpectedPortStates) {},
				setDefaultNFSGroupConfigsFunc:      func(cfgs pkgnfschecker.Configs) {},
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
				VolumePath:       tempDir,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: 5 * time.Minute},
				NumExpectedFiles: 10,
			},
		}

		// Marshal it to JSON
		configBytes, err := json.Marshal(expectedConfigs)
		assert.NoError(t, err)

		var actualConfigs pkgnfschecker.Configs
		s := &Session{
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				actualConfigs = cfgs
			},
		}

		configMap := map[string]string{
			"nfs": string(configBytes),
		}

		resp := &Response{}
		s.processUpdateConfig(configMap, resp)

		assert.Empty(t, resp.Error)
		assert.Len(t, actualConfigs, 1)
		assert.Equal(t, expectedConfigs[0].VolumePath, actualConfigs[0].VolumePath)
		assert.Equal(t, expectedConfigs[0].FileContents, actualConfigs[0].FileContents)
		assert.Equal(t, expectedConfigs[0].TTLToDelete, actualConfigs[0].TTLToDelete)
		assert.Equal(t, expectedConfigs[0].NumExpectedFiles, actualConfigs[0].NumExpectedFiles)
	})

	t.Run("nfs with multiple configs", func(t *testing.T) {
		tempDir1 := t.TempDir()
		tempDir2 := t.TempDir()

		// Create multiple GroupConfig objects
		expectedConfigs := pkgnfschecker.Configs{
			{
				VolumePath:       tempDir1,
				FileContents:     "test-content1",
				TTLToDelete:      metav1.Duration{Duration: 5 * time.Minute},
				NumExpectedFiles: 10,
			},
			{
				VolumePath:       tempDir2,
				FileContents:     "test-content2",
				TTLToDelete:      metav1.Duration{Duration: 10 * time.Minute},
				NumExpectedFiles: 20,
			},
		}

		// Marshal it to JSON
		configBytes, err := json.Marshal(expectedConfigs)
		assert.NoError(t, err)

		var actualConfigs pkgnfschecker.Configs
		s := &Session{
			setDefaultNFSGroupConfigsFunc: func(cfgs pkgnfschecker.Configs) {
				actualConfigs = cfgs
			},
		}

		configMap := map[string]string{
			"nfs": string(configBytes),
		}

		resp := &Response{}
		s.processUpdateConfig(configMap, resp)

		assert.Empty(t, resp.Error)
		assert.Len(t, actualConfigs, 2)

		// Check first config
		assert.Equal(t, expectedConfigs[0].VolumePath, actualConfigs[0].VolumePath)
		assert.Equal(t, expectedConfigs[0].FileContents, actualConfigs[0].FileContents)
		assert.Equal(t, expectedConfigs[0].TTLToDelete, actualConfigs[0].TTLToDelete)
		assert.Equal(t, expectedConfigs[0].NumExpectedFiles, actualConfigs[0].NumExpectedFiles)

		// Check second config
		assert.Equal(t, expectedConfigs[1].VolumePath, actualConfigs[1].VolumePath)
		assert.Equal(t, expectedConfigs[1].FileContents, actualConfigs[1].FileContents)
		assert.Equal(t, expectedConfigs[1].TTLToDelete, actualConfigs[1].TTLToDelete)
		assert.Equal(t, expectedConfigs[1].NumExpectedFiles, actualConfigs[1].NumExpectedFiles)
	})
}
