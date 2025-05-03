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

### Component Plugins
Component plugins run periodically to monitor system health. They can be:
- **Single Component**: One health check with one configuration
- **Multi-Component**: Multiple health checks from a single configuration, either specified directly or loaded from a file

## Configuration Format

Plugins are configured using YAML files. Here's the basic structure:

```yaml
plugin_name: string  # Required, max 128 chars
type: string        # Required, one of: "init", "component", "component_list"
run_mode: string    # Optional, defaults to "manual"
timeout: duration   # Optional, defaults to 1 minute
interval: duration  # Optional, must be >= 1 minute if specified

# For component_list type
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

When using `component_list` or `component_list_file`, each component can be specified in one of these formats:

1. **Full Format**: `name/run_mode:param`
   - `name`: Component name
   - `run_mode`: Optional run mode override
   - `param`: Optional parameter

2. **Run Mode Only**: `name/run_mode`
   - `name`: Component name
   - `run_mode`: Optional run mode override

3. **Parameter Only**: `name:param`
   - `name`: Component name
   - `param`: Parameter

4. **Name Only**: `name`
   - Just the component name

### Parameter Inheritance and Priority

When using `component_list` or `component_list_file`, certain parameters are inherited from the parent plugin specification. The priority order for these parameters is:

1. **run_mode**:
   - Highest priority: Component-specific run_mode (e.g., `name/run_mode`)
   - Middle priority: Parent plugin's run_mode
   - Default: "manual" if not specified

2. **timeout**:
   - Always inherited from parent plugin
   - Default: 1 minute if not specified in parent

3. **interval**:
   - Always inherited from parent plugin
   - Must be >= 1 minute if specified
   - Default: No interval (runs once) if not specified

Example with parameter inheritance:
```yaml
plugin_name: "multi-disk-check"
type: "component_list"
run_mode: "auto"  # Parent run_mode
timeout: 30s      # Parent timeout
interval: 5m      # Parent interval
component_list:
  - "root/auto:/"         # Uses parent timeout and interval, explicit run_mode
  - "home/manual:/home"   # Uses parent timeout and interval, overrides run_mode
  - "var:/var"            # Uses parent timeout, interval, and run_mode
```

## Component List File Format

The `component_list_file` should be a plain text file with one component per line. Each line follows the same format as described in the Component List Format section above. Empty lines and lines starting with `#` are ignored.

Example `components.txt`:
```text
# This is a comment
root/auto:/          # Full format with run_mode and param
home/auto:/home      # Full format with run_mode and param
var/auto             # Run mode only
data:param1          # Parameter only
backup               # Name only

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
- `${PAR}` or `${PAR1}` - Component parameter

## Examples

### Simple Component Plugin
```yaml
plugin_name: "disk-space-check"
type: "component"
run_mode: "manual"
timeout: 10s
health_state_plugin:
  steps:
    - name: "check-disk-space"
      run_bash_script:
        content_type: "plaintext"
        script: |
          #!/bin/bash
          df -h | grep ${NAME}
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
type: "component_list"
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
type: "component_list"
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
type: "init"
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
   - Use `component_list_file` for large numbers of components
   - Keep component names descriptive and unique
   - Use parameters to make scripts more flexible

4. **Error Handling**:
   - Always check for errors in your scripts
   - Provide meaningful error messages
   - Use appropriate exit codes

## Integration with GPUd

Plugins integrate with GPUd's component system:
- They appear in the component list
- Their health states are tracked
- They can be queried through the API
- They participate in the overall system health status

## API Access

Plugins can be accessed through GPUd's API:
- List all plugins: `GET /v1/components/custom-plugin`
- Get plugin status: `GET /v1/states?component=<plugin_name>`
- Trigger manual check: `POST /v1/components/check?componentName=<plugin_name>` 