package kernelmodule

import "testing"

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
	if err != nil {
		t.Fatalf("failed to parse /etc/modules: %v", err)
	}
	t.Logf("modules: %v", modules)
	if len(modules) != 3 {
		t.Fatalf("expected 3 modules, got %d", len(modules))
	}

	if modules[0] != "3c509 irq=15" {
		t.Fatalf("expected first module to be '3c509 irq=15', got %q", modules[0])
	}
	if modules[1] != "nf_nat_ftp" {
		t.Fatalf("expected second module to be 'nf_nat_ftp', got %q", modules[1])
	}
	if modules[2] != "w83781d" {
		t.Fatalf("expected third module to be 'w83781d', got %q", modules[2])
	}
}
