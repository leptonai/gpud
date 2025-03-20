package kernelmodule

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseEtcModules(t *testing.T) {
	// ref. https://manpages.ubuntu.com/manpages/xenial/man5/modules.5.html
	input := `
           # /etc/modules: kernel modules to load at boot time.
           #
           # This file contains the names of kernel modules that
           # should be loaded at boot time, one per line. Lines
           # beginning with "#" are ignored.

           w83781d

           3c509 irq=15
           nf_nat_ftp

`
	modules, err := parseEtcModules([]byte(input))
	assert.NoError(t, err, "failed to parse /etc/modules")
	t.Logf("modules: %v", modules)

	assert.Len(t, modules, 3, "expected 3 modules")
	assert.Equal(t, "3c509 irq=15", modules[0], "unexpected first module")
	assert.Equal(t, "nf_nat_ftp", modules[1], "unexpected second module")
	assert.Equal(t, "w83781d", modules[2], "unexpected third module")
}
