package cpu

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"k8s.io/apimachinery/pkg/api/resource"
)

// GetSystemResourceLogicalCores returns the system CPU resource of the machine
// with the logical core counts, using the type defined in "corev1.ResourceName"
// in https://pkg.go.dev/k8s.io/api/core/v1#ResourceName.
// It represents the CPU, in cores (500m = .5 cores).
// Must be parsed using the "resource.ParseQuantity" function in https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource.
func GetSystemResourceLogicalCores() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	counts, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return "", fmt.Errorf("failed to get CPU cores count: %w", err)
	}

	qty := resource.NewQuantity(int64(counts), resource.DecimalSI)
	return qty.String(), nil
}
