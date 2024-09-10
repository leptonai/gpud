package update

import (
	"fmt"
	"testing"
)

func Test_tarballName(t *testing.T) {
	ubuntuVer := detectUbuntuVersion()
	ubuntuSuffix := ""
	if ubuntuVer != "" {
		ubuntuSuffix = "_" + ubuntuVer
	}
	name := tarballName("v0.0.1", "linux", "amd64")
	want := fmt.Sprintf("gpud_v0.0.1_linux_amd64%s.tgz", ubuntuSuffix)
	if name != want {
		t.Fatalf("want: %s, got: %s", want, name)
	}
}
