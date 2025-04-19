package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGetSystemResource(t *testing.T) {
	mem, err := GetSystemResourceMemoryTotal()
	assert.NoError(t, err)

	memQty, err := resource.ParseQuantity(mem)
	assert.NoError(t, err)
	t.Logf("mem: %s", memQty.String())
}
