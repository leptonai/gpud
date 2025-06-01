# Custom Plugins

Custom plugins are a powerful way to extend GPUd's monitoring capabilities. They allow you to create custom health checks and monitoring scripts that integrate seamlessly with GPUd's component system.

## Overview

Plugins in GPUd serve two main purposes:

1. **Initialization (init)**: One-time setup tasks that run when GPUd starts
2. **Monitoring (component)**: Regular health checks that run periodically

Plugins can be configured to run as:
- Single components (one health check)
- Multiple components (multiple health checks from a single configuration)
- Initialization tasks (one-time setup)

## Plugin Types

### Init Plugins
Init plugins are used for one-time setup tasks when GPUd starts. They're perfect for:
- Setting up system configurations
- Installing required dependencies
- Initializing monitoring tools
- Setting up logging

See the Examples section (in this document below) for an example use of the init plugin.

### Component Plugins
Component plugins run periodically to monitor system health. They can be:
- **Single Component**: One health check with one configuration
- **Multi-Component**: Multiple health checks from a single configuration, either specified directly or loaded from a file

GPUd uses the exit status of plugins to determine success or failure of a plugin.  Ensure that plugins return an error code
0 for success, and non-zero for failure.  (Plugs with multiple commands may experience an error at every command.  Ensure 
that all errors are handled.  (See also **Best Practices** below.)

## Configuration Format

Plugins are configured using YAML files. Here's the basic structure:

```yaml
plugin_name: string  # Required, max 128 chars
plugin_type: string  # Required, one of: "init", "component", "component_list"
run_mode: string     # Optional, defaults to "auto"
timeout: duration    # Optional, defaults to 1 minute (e.g., "1m")
interval: duration   # Optional, must be >= 1 minute if specified

# For component_list type, specify exactly one of:
component_list: string[]  # Required for component_list type, unless component_list_file is specified
component_list_file: string  # Optional, path to file containing component list

health_state_plugin:
  steps:
    - name: string  # Required
      run_bash_script:
        content_type: string  # Required, e.g. "plaintext", "base64"
        script: string       # Required, bash script content
```

## Component List Format

Each component in the list can be specified in one of four formats:

1. **Full Format**: `name#run_mode:param`
   - `name`: Component name
   - `run_mode`: Optional run mode (possible values: auto, manual) -- "once" mode to be supported in the future
   - `param`: Optional parameter

2. **Run Mode Only**: `name#run_mode`
   - `name`: Component name
   - `run_mode`: Optional run mode (auto, manual, once)

   **Note:** Please check availability of option `once`, as `once` will be added in a future revision.

3. **Parameter Only**: `name:param`
   - `name`: Component name
   - `param`: Optional parameter

4. **Name Only**: `name`
   - Just the component name

### Parameter Inheritance and Priority

The plugin system supports parameter inheritance with the following priority order:

1. **Run Mode Priority**:
   - Highest priority: Component-specific run_mode (e.g., `name#run_mode`)
   - Middle priority: Parent plugin's run_mode
   - Lowest priority: Default run_mode (auto)

2. **Timeout and Interval**:
   - Always inherited from the parent plugin
   - No component-specific overrides

### Examples

#### Simple Component with Parameter
```yaml
plugin_name: "disk-space-check"
plugin_type: "component"
run_mode: "auto"
interval: "1m"
timeout: "30s"
steps:
  - name: "check-disk-space"
    run_bash_script:
      script: |
        #!/bin/bash
        df -h 
```

#### Component List with Parameters
```yaml
plugin_name: "multi-disk-check"
plugin_type: "component_list"
run_mode: "auto"
interval: "1m"
timeout: "30s"
component_list:
  - "/"              # Root filesystem
  - "/home"          # Home directory
  - "/var#manual"    # Manual check for /var
  - "/tmp:--inodes"  # Check inodes for /tmp
steps:
  - name: "check-disk-space"
    run_bash_script:
      script: |
        #!/bin/bash
        df -h ${PAR} | grep ${NAME}
        if [ $? -eq 0 ]; then
          echo "Disk space OK for ${NAME}"
        else
          echo "Disk space check failed for ${NAME}"
          exit 1
        fi
```

#### Component List File
```yaml
plugin_name: "multi-disk-check"
plugin_type: "component_list"
run_mode: "auto"
interval: "1m"
timeout: "30s"
component_list_file: "/etc/gpud/disk-list.txt"
steps:
  - name: "check-disk-space"
    run_bash_script:
      script: |
        #!/bin/bash
        df -h ${PAR} | grep ${NAME}
        if [ $? -eq 0 ]; then
          echo "Disk space OK for ${NAME}"
        else
          echo "Disk space check failed for ${NAME}"
          exit 1
        fi
```

Where `/etc/gpud/disk-list.txt` contains:
```
# Root filesystem
/        
# Home directory      
/home      
# Manual check for /var    
/var#manual   
# Check inodes for /tmp 
/tmp:--inodes  
```

## Component List File Format

The `component_list_file` should be a plain text file with one component per line. Each line follows the same format as described in the Component List Format section above. Empty lines and lines starting with `#` as the first character are ignored. (If you need to create a plugin with a name starting with # you could do so by indenting with a white space.)

Example `components.txt`:
```text
# This is a comment
# Full format with run_mode and param
root#auto:/        
# Full format with run_mode and param 
home#auto:/home    
# Run mode only  
var#auto        
# Parameter only     
data:param1    
# Name only      
backup               

# Another comment
```

The file is read line by line, with:
- Leading and trailing whitespace trimmed
- Empty lines skipped
- Comment lines (starting with #) skipped
- Each valid line parsed according to the Component List Format rules

## Parameter Substitution

In your bash scripts, you can use these variables:
- `${NAME}` - Component name
- `${PAR}` - Component parameter(s)

## Plugin Output and Parsing

### Purpose of Output Parsing
The output parsing system serves as a bridge between raw plugin output and actionable system responses. It:
- Extracts structured information from plugin output
- Classifies issues into categories
- Triggers appropriate remediation actions
- Provides context for operator decisions

This system is crucial for:
- Automated remediation of common issues
- Consistent handling of plugin outputs
- Clear communication of problems and solutions
- Integration with cluster management systems

### Output Format
The plugin must output its result to stdout. The output must be a valid JSON object. The output can be embedded within other text output - the parser will extract the first valid JSON object it finds.

Example JSON output:
```json
{
  "name": "plugin-name",
  "result": "success",
  "error": null,
  "passed": true,
  "runtime": "1.2s",
  "action": "reboot system",
  "suggestion": "system needs reboot",
  "commands": ["reboot"]
}
```

### Understanding JSONPath and Parser Configuration

The parser uses JSONPath syntax to extract information from the plugin output. JSONPath is a query language for JSON data, similar to XPath for XML:

- `$` represents the root of the JSON object
- `.` is the child operator
- `$.name` means "get the 'name' field from the root object"

Example JSON and corresponding JSONPath queries:
```json
{
  "name": "gpu-check",
  "result": "error",
  "error": "GPU memory overflow"
}
```
- `$.name` → "gpu-check"
- `$.result` → "error"
- `$.error` → "GPU memory overflow"

### Parser Configuration Components

The parser configuration consists of several key components:

```yaml
parser:
  json_paths:
    - query: "$.name"          # JSONPath to extract
      field: "name"            # Where to store the value
      expect:                  # Optional validation
        regex: "^gpu-check$"   # Must match pattern
      suggested_actions:       # Optional action triggers
        REBOOT_SYSTEM:         # Action name
          regex: ".*reboot.*"  # When to trigger
  log_path: "/var/log/${TRIGGER}.log"  # Optional path to log plugin output
```

#### Configuration Elements

1. **query**: The JSONPath expression to extract data
   - `$.name` - gets the name field
   - `$.result` - gets the result field
   - `$.error` - gets the error field

2. **field**: The internal name to store the extracted value
   - Used by the system to reference the value
   - Must be unique within the parser configuration

3. **expect**: Validation rules for the extracted value
   ```yaml
   expect:
     regex: "(?i)^true$"  # Case-insensitive match for "true"
   ```
   - Validates the extracted value matches the pattern
   - Can be used to ensure data quality

4. **suggested_actions**: Triggers for automated responses
   ```yaml
   suggested_actions:
     REBOOT_SYSTEM:
       regex: "(?i).*reboot.*"  # Trigger on any text containing "reboot"
   ```
   - Maps extracted values to system actions
   - Uses regex to match patterns in the output

#### Expect Rule Behavior

When an `expect` rule fails:

1. **Health State Impact**:
   - The component's health state is marked as unhealthy
   - The failure is logged with details about the expected vs actual value
   - The plugin's exit code is still considered in the overall health assessment

2. **Error Handling**:
   - The failure is logged in the component's error log
   - The error message includes:
     - The field that failed validation
     - The expected pattern
     - The actual value received
   - Any suggested actions are still evaluated and may be triggered

3. **Example Failure**:
   ```yaml
   - query: "$.result"
     field: "result"
     expect:
       regex: "^success$"
   ```
   If the plugin outputs:
   ```json
   {
     "result": "failed",
     "error": "Operation timed out"
   }
   ```
   The expect rule will fail because "failed" doesn't match "^success$", and:
   - The component will be marked unhealthy
   - The error will be logged: "Field 'result' failed validation: expected pattern '^success$', got 'failed'"
   - The plugin's exit code will still be considered

4. **Multiple Expect Rules**:
   - Each field can have its own expect rule
   - All expect rules are evaluated independently
   - A single failed expect rule marks the component as unhealthy
   - All failures are logged, not just the first one

#### System Health Assessment

The system considers both:
- The plugin's exit code (0 for success, non-zero for failure)
- The JSON output fields (result, error, passed, etc.)

This dual approach allows for:
- Quick failure detection via exit codes
- Detailed error information via JSON
- Structured error handling and remediation
- Clear communication of issues and solutions

For example, if a plugin:
1. Returns exit code 1 (failure)
2. Outputs JSON with `"result": "success"` and `"passed": true`
3. Has an expect rule requiring `"result": "success"`

The system will:
- Mark the component as unhealthy due to the non-zero exit code
- Log the JSON validation success
- Include both pieces of information in the health assessment
- Allow appropriate actions to be triggered based on the complete context

#### Error Information

Errors are captured in two ways:

1. **Plugin Exit Code**:
```bash
#!/bin/bash
if [ $? -eq 0 ]; then
  # Success case
  echo '{"result": "success", "error": null}'
  exit 0
else
  # Error case
  echo '{"result": "error", "error": "command failed"}'
  exit 1
fi
```

2. **JSON Error Information**:
```json
{
  "result": "error",           // Overall result
  "error": "GPU memory overflow", // Detailed error message
  "passed": false,            // Quick status check
  "action": "system needs reboot", // Suggested action
  "suggestion": "GPU memory exceeded limits, reboot required" // Detailed suggestion
}
```

The system considers both:
- The plugin's exit code (0 for success, non-zero for failure)
- The JSON output fields (result, error, passed, etc.)

This dual approach allows for:
- Quick failure detection via exit codes
- Detailed error information via JSON
- Structured error handling and remediation
- Clear communication of issues and solutions

#### Understanding Field Relationships

##### JSON vs GPUd Fields
- **JSON Fields**: The fields in your plugin's output JSON
  ```json
  {
    "action": "system needs reboot"  // JSON field
  }
  ```
- **GPUd Fields**: The internal names where GPUd stores extracted values
  ```yaml
  - query: "$.action"    # JSON field to extract
    field: "action"      # GPUd field to store in
  ```

##### Multiple Action Triggers
You can define multiple actions that trigger based on different conditions:

```yaml
- query: "$.action"
  field: "action"
  suggested_actions:
    REBOOT_SYSTEM:
      regex: "(?i).*reboot.*"
    HARDWARE_INSPECTION:
      regex: "(?i).*hardware.*"
    CHECK_USER_APP_AND_GPU:
      regex: "(?i).*application.*"
```

If the action field contains multiple matching terms, ALL matching actions will be triggered:
```json
{
  "action": "application error requires hardware inspection and reboot"
}
```
This would trigger:
- `REBOOT_SYSTEM` (matches "reboot")
- `HARDWARE_INSPECTION` (matches "hardware")
- `CHECK_USER_APP_AND_GPU` (matches "application")

### Real-World Example

Consider this plugin output:
```json
{
  "name": "gpu-memory-check",
  "result": "error",
  "error": "GPU memory overflow",
  "passed": false,
  "action": "system needs reboot",
  "suggestion": "GPU memory exceeded limits, reboot required",
  "commands": ["reboot"]
}
```

This output would be parsed as:
- `$.name` → "gpu-memory-check" (stored in `name` field)
- `$.result` → "error" (stored in `result` field)
- `$.error` → "GPU memory overflow" (stored in `error` field)
- `$.passed` → false (stored in `passed` field, fails `expect` rule)
- `$.action` → "system needs reboot" (stored in `action` field, triggers `REBOOT_SYSTEM`)
- `$.suggestion` → "GPU memory exceeded limits, reboot required" (stored in `suggestion` field, triggers `REBOOT_SYSTEM`)
- `$.commands` → ["reboot"] (stored in `commands` field)

The parser would:
1. Extract all these values
2. Validate the `passed` field against the expect rule
3. Trigger the `REBOOT_SYSTEM` action because both `action` and `suggestion` contain "reboot"
4. Make all this information available to the system for automated or operator-initiated remediation

### Standard Fields
The following fields are standardized across all plugins:

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `name` | string | Plugin identification | Yes |
| `result` | string | Status ("success" or "error") | Yes |
| `error` | string/null | Error information if any | Yes |
| `passed` | boolean | Quick status check | Yes |
| `runtime` | string | Execution time | No |
| `action` | string | Suggested action | No |
| `suggestion` | string | Detailed error message | No |
| `commands` | array | Specific remediation steps | No |

### Supported Repair Actions
The system supports the following standard repair actions:

| Action | Description | Use Case |
|--------|-------------|-----------|
| `REBOOT_SYSTEM` | System-level reboot | Driver issues, system hangs |
| `HARDWARE_INSPECTION` | Physical hardware check | Physical damage, hardware failures |
| `CHECK_USER_APP_AND_GPU` | Application/GPU interaction check | Application errors, GPU communication issues |
| `IGNORE_NO_ACTION_REQUIRED` | No action needed | Non-critical issues, expected behavior |

### Error Handling Best Practices

1. **Output Structure**
   - Always include `error` field (null if no error)
   - Use `result` field for status ("success" or "error")
   - Include `passed` boolean for quick status check

2. **Error Details**
   - Use `suggestion` field for actionable error messages
   - Include `commands` array for specific remediation steps
   - Set appropriate `action` field to trigger system responses

3. **Validation**
   - Use `expect` rules to validate critical fields
   - Include regex patterns for value validation
   - Define clear success/failure criteria

4. **Action Triggers**
   - Use standard repair actions as defined above
   - Ensure action triggers have clear regex patterns
   - Include appropriate context in trigger conditions

### Parser Configuration
The parser configuration in your plugin's `config.yaml` should follow this structure:

```yaml
parser:
  json_paths:
    - query: "$.result"        # JSONPath to extract
      field: "result"          # Field to store value
      expect:                  # Optional validation
        regex: "^success$"     # Must match pattern
    - query: "$.error"
      field: "error"
    - query: "$.action"
      field: "action"
      suggested_actions:       # Action triggers
        REBOOT_SYSTEM:
          regex: "(?i).*reboot.*"
  log_path: "/var/log/${PLUGIN}/${TRIGGER}.log"  # Optional path to log plugin output
```

Note: Currently, only JSON format is supported for plugin output. The parser will extract the first valid JSON object it finds in the output, even if it's embedded within other text.

### Log Path Variable Substitution

The `log_path` field in the parser configuration supports variable substitution to create dynamic log file paths. This is useful for organizing plugin output logs by plugin name and trigger type.

#### Supported Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `${PLUGIN}` | Plugin name | Yes, if used in path |
| `${TRIGGER}` | Trigger name (e.g., "auto", "manual") | Yes, if used in path |

#### Behavior

- If `${PLUGIN}` is present in the path and plugin name is empty, logging is skipped
- If `${TRIGGER}` is present in the path and trigger name is empty, logging is skipped
- Variables can be used multiple times in the same path
- Variables can be used in any part of the path (directory or filename)

#### Examples

```yaml
# Log to plugin-specific directory with trigger in filename
log_path: "/var/log/${PLUGIN}/${TRIGGER}.log"

# Log to trigger-specific directory with plugin in filename
log_path: "/var/log/${TRIGGER}/${PLUGIN}.log"

# Log to a single directory with both plugin and trigger in filename
log_path: "/var/log/plugins/${PLUGIN}_${TRIGGER}.log"

# Log to a nested structure
log_path: "/var/log/${PLUGIN}/${TRIGGER}/${PLUGIN}_${TRIGGER}.log"
```

The log entries will include:
- Timestamp in RFC3339 format
- Plugin name
- Trigger name
- Raw plugin output

Example log entry:
```
[2024-03-14T10:30:45Z] plugin=health_check trigger=auto output={"status": "healthy"}
```

Note: Currently, only JSON format is supported for plugin output. The parser will extract the first valid JSON object it finds in the output, even if it's embedded within other text.

## Examples

### Simple Component Plugin
```yaml
plugin_name: "disk-space-check"
plugin_type: "component"
run_mode: "manual"
timeout: 10s
health_state_plugin:
  steps:
    - name: "check-disk-space"
      run_bash_script:
        content_type: "plaintext"
        script: |
          #!/bin/bash
          df -h | grep /var
          if [ $? -eq 0 ]; then
            echo "Disk space OK"
            exit 0
          else
            echo "Disk space check failed"
            exit 1
          fi
```

### Multi-Component Plugin with Direct List
```yaml
plugin_name: "multi-disk-check"
plugin_type: "component_list"
component_list:
  - "root/auto:/"
  - "home/auto:/home"
  - "var/auto:/var"
health_state_plugin:
  steps:
    - name: "check-disk-space"
      run_bash_script:
        content_type: "plaintext"
        script: |
          #!/bin/bash
          df -h ${PAR} | grep ${NAME}
          if [ $? -eq 0 ]; then
            echo "Disk space OK for ${NAME}"
            exit 0
          else
            echo "Disk space check failed for ${NAME}"
            exit 1
          fi
```

### Multi-Component Plugin with File
```yaml
plugin_name: "multi-disk-check"
plugin_type: "component_list"
component_list_file: "/etc/gpud/disk_checks.txt"
health_state_plugin:
  steps:
    - name: "check-disk-space"
      run_bash_script:
        content_type: "plaintext"
        script: |
          #!/bin/bash
          df -h ${PAR} | grep ${NAME}
          if [ $? -eq 0 ]; then
            echo "Disk space OK for ${NAME}"
            exit 0
          else
            echo "Disk space check failed for ${NAME}"
            exit 1
          fi
```

### Init Plugin
```yaml
plugin_name: "setup-monitoring"
plugin_type: "init"
timeout: 5m
health_state_plugin:
  steps:
    - name: "install-tools"
      run_bash_script:
        content_type: "plaintext"
        script: |
          #!/bin/bash
          apt-get update
          apt-get install -y monitoring-tools
          systemctl enable monitoring-tools
          systemctl start monitoring-tools
```

## Best Practices

1. **Script Content**:
   - Use `content_type: "plaintext"` for readability
   - Use `content_type: "base64"` for complex scripts with special characters
   - Always include proper error handling
   - Use exit codes appropriately (0 for success, non-zero for failure)

2. **Timeouts and Intervals**:
   - Set appropriate timeouts based on script complexity
   - Use intervals >= 1 minute for component plugins
   - Consider system load when setting intervals

3. **Component Lists**:
   - Use `component_list_file` for large numbers of components that use standardized scripts (e.g., checking multiple devices, executing different health check scripts,...)
   - Keep component names descriptive and unique
   - Use parameters to make scripts more flexible

4. **Error Handling**:
   - Always check for errors in your scripts
   - Provide meaningful error messages
   - Use appropriate exit codes.

5. **Plugin scripts**
   - Plugins are executed by `/bin/bash`. You can use bash conventions, bvariables etc.
   - When running multiple commands in a plugin using bash, each command may experience an error that may lead to a corrupt result.
   - Ensure each command's error is caught and handled.  To propagate unhandled errors to GPUd, you can figure your commands with these settings:
```
            # do not mask errors
            set -o pipefail
            # treat unset variables as an error
            set -o nounset
            # exit script whenever it errs
            set -o errexit
```

## Integration with GPUd

Plugins integrate with GPUd's component system:
- They appear in the component list
- Their health states are tracked
- They can be queried through the API
- They participate in the overall system health status

## API Access

Plugins can be accessed through GPUd's API:
- List all plugins: `GET /v1/components/custom-plugin`
- Get plugin status: `GET /v1/states?components=<plugin_name>`
- Trigger manual check: `GET /v1/components/trigger-check?componentName=<plugin_name>` 

## Tags and Component Grouping

Tags provide a powerful way to group related components together and trigger them as a group. This is particularly useful for scenarios where you need to run multiple related checks or actions together, such as Slurm job prologue/epilogue scripts or continuous monitoring tasks.

### Tag Specification

Tags can be specified in two ways, with the following priority:

1. **Component List Entry Tags** (Highest Priority):
   ```yaml
   component_list:
     - "comp1#auto[slurm.prologue]"
     - "comp2#auto[slurm.prologue,slurm.continuous]"
     - "comp3#manual[slurm.epilogue]"
   ```

2. **Plugin-Level Tags** (Lower Priority):
   ```yaml
   plugin_name: "slurm-checks"
   plugin_type: "component_list"
   tags: ["slurm.continuous"]
   component_list:
     - "comp1#auto"
     - "comp2#auto"
   ```

### Triggering Components by Tag

You can trigger all components with a specific tag using the REST API:

```bash
# Trigger all components with the slurm.prologue tag
curl -X GET "http://localhost:8080/v1/components/trigger-tag?tagName=slurm.prologue"

# Trigger all components with the slurm.epilogue tag
curl -X GET "http://localhost:8080/v1/components/trigger-tag?tagName=slurm.epilogue"

# Trigger all components with the slurm.continuous tag
curl -X GET "http://localhost:8080/v1/components/trigger-tag?tagName=slurm.continuous"
```

The API response includes:
- Success status of the trigger operation
- Number of components triggered
- Exit status of the tests
- Detailed error messages if any tests failed

Example response:
```json
{
  "success": true,
  "message": "Triggered 3 components with tag: slurm.prologue",
  "exitStatus": "all tests exited with status 0"
}
```

### Example: Slurm Integration

Here's a complete example of using tags to implement Slurm prologue, epilogue, and continuous monitoring:

```yaml
plugin_name: "slurm-checks"
plugin_type: "component_list"
run_mode: "manual"
timeout: "5m"
component_list:
  # Prologue checks (run before job starts)
  - "check-gpu-availability#auto[slurm-prologue]"
  - "check-memory#auto[slurm-prologue]"
  - "check-disk-space#auto[slurm-prologue]"
  
  # Epilogue checks (run after job ends)
  - "cleanup-temp-files#auto[slurm-epilogue]"
  - "archive-logs#auto[slurm-epilogue]"
  - "release-resources#auto[slurm-epilogue]"
  
  # Continuous monitoring
  - "monitor-gpu-usage#auto[slurm-continuous]"
  - "monitor-memory-usage#auto[slurm-continuous]"
  - "monitor-disk-usage#auto[slurm-continuous]"

health_state_plugin:
  steps:
    - name: "execute-check"
      run_bash_script:
        content_type: "plaintext"
        script: |
          #!/bin/bash
          case "${NAME}" in
            "check-gpu-availability")
              nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounits | awk '{ if ($1 > 100) exit 1; exit 0 }'
              ;;
            "check-memory")
              free -g | awk '/^Mem:/ { if ($3 > 90) exit 1; exit 0 }'
              ;;
            "check-disk-space")
              df -h ${PAR} | awk 'NR==2 { if ($5 > 90) exit 1; exit 0 }'
              ;;
            "cleanup-temp-files")
              rm -rf /tmp/job-${SLURM_JOB_ID}/*
              ;;
            "archive-logs")
              tar -czf /var/log/slurm/job-${SLURM_JOB_ID}.tar.gz /tmp/job-${SLURM_JOB_ID}/logs
              ;;
            "release-resources")
              # Release any held resources
              ;;
            "monitor-gpu-usage")
              nvidia-smi --query-gpu=utilization.gpu --format=csv,noheader,nounits | awk '{ print "GPU Usage: " $1 "%" }'
              ;;
            "monitor-memory-usage")
              free -g | awk '/^Mem:/ { print "Memory Usage: " $3 "GB" }'
              ;;
            "monitor-disk-usage")
              df -h ${PAR} | awk 'NR==2 { print "Disk Usage: " $5 }'
              ;;
          esac
```

### Integration with Slurm

You can integrate these checks with Slurm by adding the appropriate trigger commands to your Slurm configuration:

1. **Prolog** (in `slurm.conf`):
```bash
Prolog=/bin/bash -c 'curl -X GET "http://localhost:8080/v1/components/trigger-tag?tagName=slurm-prolog"'
```

Alternatively, you can trigger with `gpud run-plugin-group slurm-prolog` which will return a return code of failure or success.

2. **Epilog** (in `slurm.conf`):
```bash
Epilog=/bin/bash -c 'curl -X GET "http://localhost:8080/v1/components/trigger-tag?tagName=slurm-epilog"'
```

Alternatively, you can trigger with `gpud run-plugin-group slurm-epilog` which will return a return code of failure or success.

3. **Continuous Monitoring** (in a separate script):
```bash
#!/bin/bash
while true; do
  curl -X GET "http://localhost:8080/v1/components/trigger-tag?tagName=slurm-continuous"
  sleep 300  # Check every 5 minutes
done
```

Alternatively, you can trigger with `gpud run-plugin-group slurm-continuous` which will return a return code of failure or success.

The REST API's response status can be used to implement conditional logic in your scripts as follows:

```bash
#!/bin/bash
response=$(curl -s -X GET "http://localhost:8080/v1/components/trigger-tag?tagName=slurm.prologue")
success=$(echo $response | jq -r '.success')
exit_status=$(echo $response | jq -r '.exitStatus')

if [ "$success" = "true" ] && [ "$exit_status" = "all tests exited with status 0" ]; then
  echo "All prologue checks passed"
  exit 0
else
  echo "Prologue checks failed"
  exit 1
fi
```

This allows you to:
1. Run multiple related checks together
2. Get a single success/failure status for the entire group
3. Implement conditional logic based on the test results
4. Maintain a clean separation of concerns between different types of checks 

`gpud run-plugin-group <plugin_group_name>` sets the exit code automatically.


### Understanding Field Relationships and Error Handling

### SLURM Log File Configuration

To log SLURM prolog and epilog plugin outputs to separate files, configure the parser's `log_path` in your plugin configuration:

```yaml
plugin_name: "slurm-checks"
plugin_type: "component_list"
run_mode: "manual"
timeout: "5m"
component_list:
  # Prolog checks (run before job starts)
  - "check-gpu-availability#auto[slurm-prolog]"
  - "check-memory#auto[slurm-prolog]"
  - "check-disk-space#auto[slurm-prolog]"
  
  # Epilog checks (run after job ends)
  - "cleanup-temp-files#auto[slurm-epilog]"
  - "archive-logs#auto[slurm-epilog]"
  - "release-resources#auto[slurm-epilog]"

health_state_plugin:
  steps:
    - name: "execute-check"
      run_bash_script:
        content_type: "plaintext"
        script: |
          #!/bin/bash
          case "${NAME}" in
            "check-gpu-availability")
              nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounits | awk '{ if ($1 > 100) exit 1; exit 0 }'
              ;;
            "check-memory")
              free -g | awk '/^Mem:/ { if ($3 > 90) exit 1; exit 0 }'
              ;;
            "check-disk-space")
              df -h ${PAR} | awk 'NR==2 { if ($5 > 90) exit 1; exit 0 }'
              ;;
            "cleanup-temp-files")
              rm -rf /tmp/job-${SLURM_JOB_ID}/*
              ;;
            "archive-logs")
              tar -czf /var/log/slurm/job-${SLURM_JOB_ID}.tar.gz /tmp/job-${SLURM_JOB_ID}/logs
              ;;
            "release-resources")
              # Release any held resources
              ;;
          esac
  parser:
    json_paths:
      - query: "$.result"
        field: "result"
        expect:
          regex: "^success$"
    log_path: "/var/log/${TRIGGER}.log"  # Creates separate log files for prolog and epilog
```

This configuration will create two log files:
- `/var/log/slurm-prolog.log` - Contains all prolog check outputs
- `/var/log/slurm-epilog.log` - Contains all epilog check outputs

Example log entries:
```
# In /var/log/slurm-prolog.log
[2024-03-14T10:30:45Z] plugin=slurm-checks trigger=prolog output={"result": "success", "name": "check-gpu-availability"}
[2024-03-14T10:30:46Z] plugin=slurm-checks trigger=prolog output={"result": "success", "name": "check-memory"}

# In /var/log/slurm-epilog.log
[2024-03-14T11:30:45Z] plugin=slurm-checks trigger=epilog output={"result": "success", "name": "cleanup-temp-files"}
[2024-03-14T11:30:46Z] plugin=slurm-checks trigger=epilog output={"result": "success", "name": "archive-logs"}
```

The REST API's response status can be used to implement conditional logic in your scripts:

###### Examples: test plugins locally

To validate sample/example configuration

```bash
gpud cs
```

```text
+-----------------------------------+-----------+----------+---------+----------+---------+
|             COMPONENT             |   TYPE    | RUN MODE | TIMEOUT | INTERVAL |  VALID  |
+-----------------------------------+-----------+----------+---------+----------+---------+
|           test-healthy            | component |   auto   |  1m0s   |  10m0s   | ✔ valid |
+-----------------------------------+-----------+----------+---------+----------+---------+
|          test-unhealthy           | component |   auto   |  1m0s   |  10m0s   | ✔ valid |
+-----------------------------------+-----------+----------+---------+----------+---------+
| test-unhealthy-with-missing-field | component |   auto   |  1m0s   |  10m0s   | ✔ valid |
+-----------------------------------+-----------+----------+---------+----------+---------+
|              exit-0               | component |   auto   |  1m0s   | 1h40m0s  | ✔ valid |
+-----------------------------------+-----------+----------+---------+----------+---------+
|              exit-1               | component |   auto   |  1m0s   | 1h40m0s  | ✔ valid |
+-----------------------------------+-----------+----------+---------+----------+---------+
```

To run and see the results of the sample configuration:

```bash
gpud cs -r
```

```text
### Component "test-healthy" output

hello world no JSON yet
{"name": "test", "result": "healthy", "passed": true, "action": "reboot me 1", "suggestion": "reboot me 2"}
thank you



### Component "test-unhealthy" output

hello world no JSON yet
{"name": "test", "result": "unhealthy", "passed": false, "action": "reboot me 1", "suggestion": "reboot me 2"}
done



### Component "test-unhealthy-with-missing-field" output

hello world no JSON yet
{"name": "test", "result": "unhealthy", "passed": false}
done



### Component "exit-0" output

{"description": "calling exit 0"}



### Component "exit-1" output

{"description": "calling exit 1"}



### Results

+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
|             COMPONENT             | HEALTH STATE |                   SUMMARY                   |     ERROR     | RUN MODE |                                               EXTRA INFO                                                |
+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
|           test-healthy            |  ✔ Healthy   |                     ok                      |               |   auto   |  {"action":"reboot me 1","name":"test","passed":"true","result":"healthy","suggestion":"reboot me 2"}   |
+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
|          test-unhealthy           | ✘ Unhealthy  |          unexpected plugin output           |               |   auto   | {"action":"reboot me 1","name":"test","passed":"false","result":"unhealthy","suggestion":"reboot me 2"} |
+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
| test-unhealthy-with-missing-field | ✘ Unhealthy  |          unexpected plugin output           |               |   auto   |                   {"name":"test","nothere":"","passed":"false","result":"unhealthy"}                    |
+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
|              exit-0               |  ✔ Healthy   |                     ok                      |               |   auto   |                                    {"description":"calling exit 0"}                                     |
+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
|              exit-1               | ✘ Unhealthy  | error executing state plugin (exit code: 1) | exit status 1 |   auto   |                                    {"description":"calling exit 1"}                                     |
+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
```

To validate and run your own configuration:

```bash
gpud cs ./pkg/custom-plugins/testdata/plugins.plaintext.2.regex.yaml
gpud cs ./pkg/custom-plugins/testdata/plugins.plaintext.2.regex.yaml -r
```

To start GPUd with the local plugin configuration file:

```bash
gpud run \
--enable-auto-update=false \
--plugin-specs-file=./pkg/custom-plugins/testdata/plugins.plaintext.2.regex.yaml
```

To trigger the registered plugin manually (works for any component):

```bash
curl -s -kL https://localhost:15132/v1/components/trigger-check?componentName=exit-1 | jq
```

```json
[
  {
    "time": "...",
    "component": "exit-1",
    "component_type": "custom-plugin",
    "name": "exit-1",
    "run_mode": "auto",
    "health": "Unhealthy",
    "reason": "error executing state plugin (exit code: 1)",
    "error": "exit status 1",
    "suggested_actions": {
      "description": "reboot me",
      "repair_actions": [
        "REBOOT_SYSTEM"
      ]
    },
    "extra_info": {
      "action": "reboot me",
      "description": "about to fail with exit code 1"
    }
  }
]
```

Other endpoints:

```bash
curl -kL https://localhost:15132/healthz
curl -kL https://localhost:15132/v1/states | jq | less
curl -kL https://localhost:15132/v1/metrics | jq | less
curl -kL https://localhost:15132/v1/events | jq | less
curl -kL https://localhost:15132/metrics
```
