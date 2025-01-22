package nvidia_smi

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components/accelerator/nvidia/query"
)

func TestMock(t *testing.T) {
	err := Mock(NormalSMIOutput, NormalQueryOutput)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gotSMIOutput, err := query.RunSMI(ctx, []string{"nvidia-smi"})
	assert.NoError(t, err)

	assert.Equal(t, string(gotSMIOutput), NormalSMIOutput)
}
