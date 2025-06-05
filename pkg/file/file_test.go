package file

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocateExecutable(t *testing.T) {
	execPath, err := LocateExecutable("ls")
	require.NoError(t, err, "LocateExecutable() should not fail")
	assert.NotEmpty(t, execPath, "executable path should not be empty")
	t.Logf("found executable %q", execPath)
}
