package container

import "testing"

func TestIsDockerRunning(t *testing.T) {
	t.Logf("%v", IsDockerRunning())
}
