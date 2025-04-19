package customplugins

import "strings"

const ComponentPrefix = "custom-plugin-"

// ConvertToComponentName converts the plugin name to a component name.
// It replaces all whitespace characters with underscores.
func ConvertToComponentName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}

	name = strings.ReplaceAll(name, " ", "-")
	if !strings.HasPrefix(name, ComponentPrefix) {
		name = ComponentPrefix + name
	}
	return name
}
