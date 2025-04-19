package cpu

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGetSystemResource(t *testing.T) {
	cpu, err := GetSystemResourceLogicalCores()
	assert.NoError(t, err)

	cpuQty, err := resource.ParseQuantity(cpu)
	assert.NoError(t, err)
	t.Logf("cpu: %s", cpuQty.String())
}
