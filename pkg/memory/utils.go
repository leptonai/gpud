package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
	"k8s.io/apimachinery/pkg/api/resource"
)

// GetSystemResourceMemoryTotal returns the system memory resource of the machine
// for the total memory size, using the type defined in "corev1.ResourceName"
// in https://pkg.go.dev/k8s.io/api/core/v1#ResourceName.
// It represents the Memory, in bytes (500Gi = 500GiB = 500 * 1024 * 1024 * 1024).
// Must be parsed using the "resource.ParseQuantity" function in https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource.
func GetSystemResourceMemoryTotal() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get memory: %w", err)
	}

	qty := resource.NewQuantity(int64(vm.Total), resource.DecimalSI)
	return qty.String(), nil
}
