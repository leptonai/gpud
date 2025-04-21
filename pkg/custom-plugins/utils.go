package customplugins

import "strings"

// ConvertToComponentName converts the plugin name to a component name.
// It replaces all whitespace characters with underscores.
func ConvertToComponentName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}

	name = strings.ReplaceAll(name, " ", "-")
	return name
}
