### Plugin Output
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

### Output Handling and Parsing

#### Standard Fields
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

#### Supported Repair Actions
The system supports the following standard repair actions:

| Action | Description | Use Case |
|--------|-------------|-----------|
| `REBOOT_SYSTEM` | System-level reboot | Driver issues, system hangs |
| `HARDWARE_INSPECTION` | Physical hardware check | Physical damage, hardware failures |
| `CHECK_USER_APP_AND_GPU` | Application/GPU interaction check | Application errors, GPU communication issues |
| `IGNORE_NO_ACTION_REQUIRED` | No action needed | Non-critical issues, expected behavior |

#### Error Handling Best Practices

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

#### Parser Configuration
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
```

Note: Currently, only JSON format is supported for plugin output. The parser will extract the first valid JSON object it finds in the output, even if it's embedded within other text.

### Plugin Configuration 