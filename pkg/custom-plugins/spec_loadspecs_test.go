package customplugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSpecsWithComponentListFile(t *testing.T) {
	// Test loading the actual testdata file
	testdataPath := filepath.Join("testdata", "plugins.component_list.yaml")

	// Check if file exists
	if _, err := os.Stat(testdataPath); os.IsNotExist(err) {
		t.Skip("Testdata file not found, skipping test")
	}

	specs, err := LoadSpecs(testdataPath)
	require.NoError(t, err)

	// Should have 2 specs: 1 init plugin + 1 expanded component
	require.Len(t, specs, 2, "Expected 2 specs after expansion")

	// First should be the init plugin
	initSpec := specs[0]
	assert.Equal(t, "nv-plugin-install-test-custom-checks", initSpec.PluginName)
	assert.Equal(t, SpecTypeInit, initSpec.PluginType)
	assert.Equal(t, "auto", initSpec.RunMode)
	assert.Equal(t, 5*time.Minute, initSpec.Timeout.Duration)

	// Second should be the expanded component from the component_list
	expandedSpec := specs[1]

	// Verify properties of the expanded component
	assert.Equal(t, "health_checks.hw_drv_gpu_clock_idle", expandedSpec.PluginName)
	assert.Equal(t, SpecTypeComponent, expandedSpec.PluginType)
	assert.Equal(t, "auto", expandedSpec.RunMode)
	assert.Equal(t, []string{"gpu", "my-plugin"}, expandedSpec.Tags)
	assert.Equal(t, 2*time.Minute, expandedSpec.Timeout.Duration)
	assert.Equal(t, 15*time.Minute, expandedSpec.Interval.Duration)

	// Verify parameter substitution in the script
	require.NotNil(t, expandedSpec.HealthStatePlugin)
	require.Len(t, expandedSpec.HealthStatePlugin.Steps, 1)
	require.NotNil(t, expandedSpec.HealthStatePlugin.Steps[0].RunBashScript)

	script := expandedSpec.HealthStatePlugin.Steps[0].RunBashScript.Script
	// Check that ${NAME} and ${PAR} were replaced
	assert.Contains(t, script, "python3 -m health_checks.hw_drv_gpu_clock_idle --idle_util 70 --idle_mem 70")
	assert.NotContains(t, script, "${NAME}", "NAME variable should have been substituted")
	assert.NotContains(t, script, "${PAR}", "PAR variable should have been substituted")

	// Verify the script structure is correct
	assert.Contains(t, script, "cat > /tmp/my-plugin-health_checks.hw_drv_gpu_clock_idle.bash")
	assert.Contains(t, script, "cd /var/local/test-custom-checks")
	assert.Contains(t, script, "source /var/local/test-custom-checks/.venv/bin/activate")

	// Verify parser configuration was preserved
	require.NotNil(t, expandedSpec.HealthStatePlugin.Parser)
	require.NotNil(t, expandedSpec.HealthStatePlugin.Parser.JSONPaths)
	require.Len(t, expandedSpec.HealthStatePlugin.Parser.JSONPaths, 8, "Expected 8 JSON paths in parser")

	// Check for the "passed" field with regex expectation
	var passedFieldFound bool
	for _, jsonPath := range expandedSpec.HealthStatePlugin.Parser.JSONPaths {
		if jsonPath.Field == "passed" {
			passedFieldFound = true
			require.NotNil(t, jsonPath.Expect)
			require.NotNil(t, jsonPath.Expect.Regex)
			assert.Equal(t, "(?i)^true$", *jsonPath.Expect.Regex)
			break
		}
	}
	assert.True(t, passedFieldFound, "Expected to find 'passed' field in JSON paths")

	// Verify suggested actions were preserved
	var actionFieldFound, suggestionFieldFound bool
	for _, jsonPath := range expandedSpec.HealthStatePlugin.Parser.JSONPaths {
		if jsonPath.Field == "action" {
			actionFieldFound = true
			require.NotNil(t, jsonPath.SuggestedActions)
			require.NotNil(t, jsonPath.SuggestedActions["REBOOT_SYSTEM"].Regex)
			assert.Equal(t, "(?i).*reboot.*", *jsonPath.SuggestedActions["REBOOT_SYSTEM"].Regex)
		}
		if jsonPath.Field == "suggestion" {
			suggestionFieldFound = true
			require.NotNil(t, jsonPath.SuggestedActions)
			require.NotNil(t, jsonPath.SuggestedActions["REBOOT_SYSTEM"].Regex)
			assert.Equal(t, "(?i).*reboot.*", *jsonPath.SuggestedActions["REBOOT_SYSTEM"].Regex)
		}
	}
	assert.True(t, actionFieldFound, "Expected to find 'action' field in JSON paths")
	assert.True(t, suggestionFieldFound, "Expected to find 'suggestion' field in JSON paths")
}

func TestLoadSpecsWithComponentListFromFile(t *testing.T) {
	// Test component_list_file functionality
	tmpDir, err := os.MkdirTemp("", "loadspecs-file-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a component list file
	componentsPath := filepath.Join(tmpDir, "components.txt")
	componentsContent := `# Component list file
health_checks.cpu_check:--threshold 80
health_checks.memory_check#manual:--threshold 90

# GPU checks with tags
health_checks.gpu_temp#auto[gpu,critical]:--max_temp 85
health_checks.gpu_util[gpu]:--max_util 95
`
	err = os.WriteFile(componentsPath, []byte(componentsContent), 0644)
	require.NoError(t, err)

	// Create YAML that references the file
	yamlContent := `
- plugin_name: file-based-health-checks
  plugin_type: component_list
  run_mode: auto
  timeout: 2m
  interval: 10m
  tags: [monitoring]
  component_list_file: ` + componentsPath + `
  health_state_plugin:
    steps:
      - name: Run health check
        run_bash_script:
          content_type: plaintext
          script: |
            echo "Running ${NAME} with params: ${PAR}"
            python3 -m ${NAME} ${PAR}
`

	yamlPath := filepath.Join(tmpDir, "test-plugins.yaml")
	err = os.WriteFile(yamlPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// Load and verify
	specs, err := LoadSpecs(yamlPath)
	require.NoError(t, err)
	require.Len(t, specs, 4, "Expected 4 components from file")

	// Verify each component
	tests := []struct {
		name    string
		runMode string
		tags    []string
		params  string
	}{
		{"health_checks.cpu_check", "auto", []string{"monitoring"}, "--threshold 80"},
		{"health_checks.memory_check", "manual", []string{"monitoring"}, "--threshold 90"},
		{"health_checks.gpu_temp", "auto", []string{"gpu", "critical"}, "--max_temp 85"},
		{"health_checks.gpu_util[gpu]", "auto", []string{"monitoring"}, "--max_util 95"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := specs[i]
			assert.Equal(t, tt.name, spec.PluginName)
			assert.Equal(t, tt.runMode, spec.RunMode)
			assert.Equal(t, tt.tags, spec.Tags)

			script := spec.HealthStatePlugin.Steps[0].RunBashScript.Script
			expectedName := tt.name
			if strings.Contains(tt.name, "[") {
				// For components with tags in the name, the whole name including tags is used
				expectedName = tt.name
			}
			assert.Contains(t, script, "Running "+expectedName+" with params: "+tt.params)
			assert.NotContains(t, script, "${NAME}")
			assert.NotContains(t, script, "${PAR}")
		})
	}
}

func TestLoadSpecsComponentListEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		yamlContent   string
		expectedError string
	}{
		{
			name: "component list with both list and file",
			yamlContent: `
- plugin_name: invalid-config
  plugin_type: component_list
  run_mode: auto
  component_list:
    - comp1
  component_list_file: /some/file.txt
  health_state_plugin:
    steps:
      - name: Run
        run_bash_script:
          content_type: plaintext
          script: echo "${NAME}"
`,
			expectedError: "component list must be empty when using component_list_file",
		},
		{
			name: "empty component list",
			yamlContent: `
- plugin_name: empty-list
  plugin_type: component_list
  run_mode: auto
  component_list: []
  health_state_plugin:
    steps:
      - name: Run
        run_bash_script:
          content_type: plaintext
          script: echo "${NAME}"
`,
			expectedError: "component list is empty",
		},
		{
			name: "invalid tag format",
			yamlContent: `
- plugin_name: invalid-tags
  plugin_type: component_list
  run_mode: auto
  component_list:
    - "comp1#manual[unclosed"
  health_state_plugin:
    steps:
      - name: Run
        run_bash_script:
          content_type: plaintext
          script: echo "${NAME}"
`,
			expectedError: "invalid tag format",
		},
		{
			name: "component list file not found",
			yamlContent: `
- plugin_name: file-not-found
  plugin_type: component_list
  run_mode: auto
  component_list_file: /non/existent/file.txt
  health_state_plugin:
    steps:
      - name: Run
        run_bash_script:
          content_type: plaintext
          script: echo "${NAME}"
`,
			expectedError: "failed to read component list file",
		},
		{
			name: "wrong plugin type with component_list",
			yamlContent: `
- plugin_name: wrong-type
  plugin_type: component  # Wrong! Should be component_list
  run_mode: auto
  component_list:
    - comp1:param1
  health_state_plugin:
    steps:
      - name: Run
        run_bash_script:
          content_type: plaintext
          script: echo "NAME=${NAME} PAR=${PAR}"
`,
			expectedError: "", // This won't error, but NAME/PAR won't be substituted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "loadspecs-edge-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			yamlPath := filepath.Join(tmpDir, "test-plugins.yaml")
			err = os.WriteFile(yamlPath, []byte(tt.yamlContent), 0644)
			require.NoError(t, err)

			specs, err := LoadSpecs(yamlPath)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else if tt.name == "wrong plugin type with component_list" {
				// Special case: wrong type doesn't error but doesn't expand
				require.NoError(t, err)
				require.Len(t, specs, 1)
				script := specs[0].HealthStatePlugin.Steps[0].RunBashScript.Script
				assert.Contains(t, script, "${NAME}", "Variables not substituted with wrong plugin_type")
				assert.Contains(t, script, "${PAR}", "Variables not substituted with wrong plugin_type")
			}
		})
	}
}

func TestParseComponentListEntrySimplified(t *testing.T) {
	tests := []struct {
		input    string
		name     string
		param    string
		runMode  string
		tags     []string
		hasError bool
	}{
		{
			input:   "simple_component",
			name:    "simple_component",
			param:   "",
			runMode: "",
			tags:    nil,
		},
		{
			input:   "component:param_value",
			name:    "component",
			param:   "param_value",
			runMode: "",
			tags:    nil,
		},
		{
			input:   "component#manual",
			name:    "component",
			param:   "",
			runMode: "manual",
			tags:    nil,
		},
		{
			input:   "component#auto[tag1,tag2]",
			name:    "component",
			param:   "",
			runMode: "auto",
			tags:    []string{"tag1", "tag2"},
		},
		{
			input:   "health_checks.hw_drv_gpu_clock_idle#auto[gpu,my-plugin]:--idle_util 70 --idle_mem 70",
			name:    "health_checks.hw_drv_gpu_clock_idle",
			param:   "--idle_util 70 --idle_mem 70",
			runMode: "auto",
			tags:    []string{"gpu", "my-plugin"},
		},
		{
			input:   "component[tag1,tag2]:param",
			name:    "component[tag1,tag2]", // Tags without runmode become part of the name
			param:   "param",
			runMode: "",
			tags:    nil,
		},
		{
			input:   "component#manual[tag1, tag2, tag3]:--option value",
			name:    "component",
			param:   "--option value",
			runMode: "manual",
			tags:    []string{"tag1", "tag2", "tag3"},
		},
		{
			input:    "",
			hasError: true,
		},
		{
			input:   "component[unclosed",
			name:    "component[unclosed", // Without # prefix, brackets are part of the name
			param:   "",
			runMode: "",
			tags:    nil,
		},
		{
			input:   "component]unopened",
			name:    "component]unopened", // Without # prefix, brackets are part of the name
			param:   "",
			runMode: "",
			tags:    nil,
		},
		{
			input:    "component#manual[unclosed", // This should error - unclosed bracket in run mode
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, param, runMode, tags, err := parseComponentListEntry(tt.input)

			if tt.hasError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.name, name)
			assert.Equal(t, tt.param, param)
			assert.Equal(t, tt.runMode, runMode)
			assert.Equal(t, tt.tags, tags)
		})
	}
}
