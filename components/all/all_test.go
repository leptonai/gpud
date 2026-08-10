package all

import "testing"

func TestAllExcludesKubelet(t *testing.T) {
	for _, component := range All() {
		if component.Name == "kubelet" {
			t.Fatal("kubelet component must not be registered")
		}
	}
}
