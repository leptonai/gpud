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

## Configuration Format

Plugins are configured using YAML files. Here's the basic structure:

```yaml
plugin_name: string  # Required, max 128 chars
type: string        # Required, one of: "init", "component", "component_list"
run_mode: string    # Optional, defaults to "auto"
timeout: duration   # Optional, defaults to 1 minute (e.g., "1m")
interval: duration  # Optional, must be >= 1 minute if specified

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
type: "component"
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
type: "component_list"
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
type: "component_list"
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
- Get plugin status: `GET /v1/states?components=<plugin_name>`
- Trigger manual check: `GET /v1/components/trigger-check?componentName=<plugin_name>` 

## Labels and Component Grouping

Labels provide a powerful way to group related components together and trigger them as a group. This is particularly useful for scenarios where you need to run multiple related checks or actions together, such as Slurm job prologue/epilogue scripts or continuous monitoring tasks.

### Label Specification

Labels can be specified in two ways, with the following priority:

1. **Component List Entry Labels** (Highest Priority):
   ```yaml
   component_list:
     - "comp1#auto[slurm.prologue]"
     - "comp2#auto[slurm.prologue,slurm.continuous]"
     - "comp3#manual[slurm.epilogue]"
   ```

2. **Plugin-Level Labels** (Lower Priority):
   ```yaml
   plugin_name: "slurm-checks"
   type: "component_list"
   labels: ["slurm.continuous"]
   component_list:
     - "comp1#auto"
     - "comp2#auto"
   ```

### Triggering Components by Label

You can trigger all components with a specific label using the REST API:

```bash
# Trigger all components with the slurm.prologue label
curl -X GET "http://localhost:8080/v1/components/trigger-label?labelName=slurm.prologue"

# Trigger all components with the slurm.epilogue label
curl -X GET "http://localhost:8080/v1/components/trigger-label?labelName=slurm.epilogue"

# Trigger all components with the slurm.continuous label
curl -X GET "http://localhost:8080/v1/components/trigger-label?labelName=slurm.continuous"
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
  "message": "Triggered 3 components with label: slurm.prologue",
  "exitStatus": "all tests exited with status 0"
}
```

### Example: Slurm Integration

Here's a complete example of using labels to implement Slurm prologue, epilogue, and continuous monitoring:

```yaml
plugin_name: "slurm-checks"
type: "component_list"
run_mode: "manual"
timeout: "5m"
component_list:
  # Prologue checks (run before job starts)
  - "check-gpu-availability#auto[slurm.prologue]"
  - "check-memory#auto[slurm.prologue]"
  - "check-disk-space#auto[slurm.prologue]"
  
  # Epilogue checks (run after job ends)
  - "cleanup-temp-files#auto[slurm.epilogue]"
  - "archive-logs#auto[slurm.epilogue]"
  - "release-resources#auto[slurm.epilogue]"
  
  # Continuous monitoring
  - "monitor-gpu-usage#auto[slurm.continuous]"
  - "monitor-memory-usage#auto[slurm.continuous]"
  - "monitor-disk-usage#auto[slurm.continuous]"

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

1. **Prologue** (in `slurm.conf`):
```bash
Prolog=/bin/bash -c 'curl -X GET "http://localhost:8080/v1/components/trigger-label?labelName=slurm.prologue"'
```

2. **Epilogue** (in `slurm.conf`):
```bash
Epilog=/bin/bash -c 'curl -X GET "http://localhost:8080/v1/components/trigger-label?labelName=slurm.epilogue"'
```

3. **Continuous Monitoring** (in a separate script):
```bash
#!/bin/bash
while true; do
  curl -X GET "http://localhost:8080/v1/components/trigger-label?labelName=slurm.continuous"
  sleep 300  # Check every 5 minutes
done
```

The REST API's response status can be used to implement conditional logic in your scripts:

```bash
#!/bin/bash
response=$(curl -s -X GET "http://localhost:8080/v1/components/trigger-label?labelName=slurm.prologue")
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