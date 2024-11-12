package systemd

import (
	"reflect"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		version       string
		expectedVer   string
		expectedExtra []string
	}{
		{
			name:          "Empty version",
			version:       "",
			expectedVer:   "",
			expectedExtra: nil,
		},
		{
			name:          "Single line version",
			version:       "1.2.3",
			expectedVer:   "1.2.3",
			expectedExtra: []string{},
		},
		{
			name:          "Multi-line version",
			version:       "   \n\n  4.5.6\n\n\n",
			expectedVer:   "4.5.6",
			expectedExtra: []string{},
		},
		{
			name:          "Version with leading/trailing spaces",
			version:       "   7.8.9   ",
			expectedVer:   "7.8.9",
			expectedExtra: []string{},
		},
		{
			name:        "Version example",
			version:     "systemd 249 (249.11-0ubuntu3.12)\n+PAM +AUDIT +SELINUX +APPARMOR +IMA +SMACK +SECCOMP +GCRYPT +GNUTLS +OPENSSL +ACL +BLKID +CURL +ELFUTILS +FIDO2 +IDN2 -IDN +IPTC +KMOD +LIBCRYPTSETUP +LIBFDISK +PCRE2 -PWQUALITY -P11KIT -QRENCODE +BZIP2 +LZ4 +XZ +ZLIB +ZSTD -XKBCOMMON +UTMP +SYSVINIT default-hierarchy=unified\n",
			expectedVer: "systemd 249 (249.11-0ubuntu3.12)",
			expectedExtra: []string{
				"+PAM +AUDIT +SELINUX +APPARMOR +IMA +SMACK +SECCOMP +GCRYPT +GNUTLS +OPENSSL +ACL +BLKID +CURL +ELFUTILS +FIDO2 +IDN2 -IDN +IPTC +KMOD +LIBCRYPTSETUP +LIBFDISK +PCRE2 -PWQUALITY -P11KIT -QRENCODE +BZIP2 +LZ4 +XZ +ZLIB +ZSTD -XKBCOMMON +UTMP +SYSVINIT default-hierarchy=unified",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, extra := parseVersion(test.version)
			if result != test.expectedVer {
				t.Errorf("unexpected result - got: %s, expected: %s", result, test.expectedVer)
			}
			if !reflect.DeepEqual(extra, test.expectedExtra) {
				t.Errorf("unexpected extra - got: %s, expected: %s", result, test.expectedExtra)
			}
		})
	}
}

func TestParseSystemdUnitUptime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expected    time.Duration
		expectedErr bool
	}{
		{
			name:        "Valid input",
			input:       "Wed 2024-02-28 01:29:39 UTC\n",
			expectedErr: false,
		},
		{
			name:        "Valid input with trailing char",
			input:       "Wed 2024-02-28 01:29:39 UTC\x0a",
			expectedErr: false,
		},
		{
			name:        "Valid input - Saturday",
			input:       "Sat 2024-11-02 13:51:36 UTC\n",
			expectedErr: false,
		},
		{
			name:        "Invalid input",
			input:       "Invalid input",
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSystemdUnitUptime(test.input)
			if (err != nil) != test.expectedErr {
				t.Errorf("unexpected error - got: %v, expected: %v", err, test.expectedErr)
			}
		})
	}
}
