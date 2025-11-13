package product

import (
	"regexp"
	"strings"
)

var (
	productNameSanitizerRegex  = "[^A-Za-z0-9-_. ]"
	productNameSanitizerRegexp = regexp.MustCompile(productNameSanitizerRegex)
)

// SanitizeProductName sanitizes the product name as in NVIDIA device plugin.
//
// e.g.,
// "NVIDIA H100 80GB HBM3" becomes "NVIDIA-H100-80GB-HBM3"
//
// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/f666bc3f836a09ae2fda439f3d7a8d8b06b48ac4/internal/lm/resource.go#L187-L204
// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/f666bc3f836a09ae2fda439f3d7a8d8b06b48ac4/internal/lm/resource.go#L314-L322
func SanitizeProductName(productName string) string {
	productName = strings.TrimSpace(productName)
	productName = productNameSanitizerRegexp.ReplaceAllString(productName, "")
	return strings.Join(strings.Fields(productName), "-")
}
