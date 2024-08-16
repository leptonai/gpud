package update

import (
	"testing"
)

func Test_tarballName(t *testing.T) {
	name := tarballName("v0.0.1", "linux", "amd64")
	want := "gpud_v0.0.1_linux_amd64.tgz"
	if name != want {
		t.Fatalf("want: %s, got: %s", want, name)
	}
}
