package lspci

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components/accelerator/nvidia/query"
)

func TestMock(t *testing.T) {
	err := Mock(NormalOutput)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	deviceNames, err := query.ListNVIDIAPCIs(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"06:00.0 3D controller: NVIDIA Corporation GA102GL [A10] (rev a1)"}, deviceNames)
}
