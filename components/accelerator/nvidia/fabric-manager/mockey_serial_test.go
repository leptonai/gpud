package fabricmanager

import (
	"sync"
	"testing"
)

var mockeyPatchMu sync.Mutex

func lockMockeyPatch(t *testing.T) {
	t.Helper()
	mockeyPatchMu.Lock()
	t.Cleanup(func() {
		mockeyPatchMu.Unlock()
	})
}
