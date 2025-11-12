package asn_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/asn"
	"github.com/leptonai/gpud/pkg/netutil"
)

func TestGetASLookupWithPublicIP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ASN integration test in short mode")
	}
	if os.Getenv("RUN_ASN_INTEGRATION") == "" {
		t.Skip("set RUN_ASN_INTEGRATION=1 to run ASN integration tests")
	}

	ip, err := netutil.PublicIP()
	require.NoError(t, err, "failed to discover public IP")

	lookup, err := asn.GetASLookup(ip)
	require.NoError(t, err, "GetASLookup failed for IP %s", ip)
	require.NotNil(t, lookup, "GetASLookup returned nil response for IP %s", ip)

	assert.NotEmpty(t, lookup.Asn, "GetASLookup returned empty ASN for IP %s", ip)
	assert.NotEmpty(t, lookup.AsnName, "GetASLookup returned empty ASN name for IP %s", ip)
	assert.Equal(t, ip, lookup.IP, "GetASLookup returned different IP than requested")

	t.Logf("ASN lookup successful for IP %s: ASN=%s, Name=%s, Country=%s, Range=%s",
		ip, lookup.Asn, lookup.AsnName, lookup.Country, lookup.AsnRange)
}

func TestFetchASLookupHackerTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ASN integration test in short mode")
	}
	if os.Getenv("RUN_ASN_INTEGRATION") == "" {
		t.Skip("set RUN_ASN_INTEGRATION=1 to run ASN integration tests")
	}

	// Test with known public IPs from different providers
	testCases := []struct {
		name        string
		ip          string
		expectASN   bool // Whether we expect a valid ASN
		expectName  bool // Whether we expect a non-empty name
		expectError bool // Whether we expect an error
	}{
		{
			name:        "Google DNS",
			ip:          "8.8.8.8",
			expectASN:   true,
			expectName:  true,
			expectError: false,
		},
		{
			name:        "Cloudflare DNS",
			ip:          "1.1.1.1",
			expectASN:   true,
			expectName:  true,
			expectError: false,
		},
		{
			name:        "Invalid IP",
			ip:          "256.256.256.256",
			expectASN:   false,
			expectName:  false,
			expectError: false, // HackerTarget returns empty result, not error
		},
	}

	for _, tc := range testCases {
		t.Run("HackerTarget_"+tc.name, func(t *testing.T) {
			// Directly test HackerTarget API
			lookup, err := asn.FetchASLookupHackerTarget(tc.ip)

			if tc.expectError {
				assert.Error(t, err, "FetchASLookupHackerTarget expected error for %s", tc.ip)
				return
			}

			if err != nil {
				// HackerTarget may have rate limits or be unavailable
				if strings.Contains(err.Error(), "rate limit") || strings.Contains(err.Error(), "timeout") {
					t.Skipf("HackerTarget API unavailable or rate limited for %s: %v", tc.ip, err)
				}
				require.NoError(t, err, "FetchASLookupHackerTarget returned unexpected error for %s", tc.ip)
			}

			require.NotNil(t, lookup, "FetchASLookupHackerTarget returned nil for %s", tc.ip)

			if tc.expectASN {
				assert.NotEmpty(t, lookup.Asn, "FetchASLookupHackerTarget returned empty ASN for %s", tc.ip)
			}

			if tc.expectName && lookup.AsnName == "" {
				t.Logf("Warning: FetchASLookupHackerTarget(%s) returned empty ASN name (may trigger fallback)", tc.ip)
			}

			t.Logf("HackerTarget lookup for %s: ASN=%s, Name=%s, Country=%s, Range=%s",
				tc.ip, lookup.Asn, lookup.AsnName, lookup.Country, lookup.AsnRange)
		})
	}
}

func TestFetchASLookupTeamCymru(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ASN integration test in short mode")
	}
	if os.Getenv("RUN_ASN_INTEGRATION") == "" {
		t.Skip("set RUN_ASN_INTEGRATION=1 to run ASN integration tests")
	}

	// Test with known public IPs
	testCases := []struct {
		name        string
		ip          string
		expectASN   bool
		expectName  bool
		expectError bool
	}{
		{
			name:        "Google DNS",
			ip:          "8.8.8.8",
			expectASN:   true,
			expectName:  true,
			expectError: false,
		},
		{
			name:        "Cloudflare DNS",
			ip:          "1.1.1.1",
			expectASN:   true,
			expectName:  true,
			expectError: false,
		},
		{
			name:        "AWS IP",
			ip:          "52.94.76.131", // Known AWS IP
			expectASN:   true,
			expectName:  true,
			expectError: false,
		},
		{
			name:        "Invalid IP",
			ip:          "not-an-ip",
			expectASN:   false,
			expectName:  false,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run("TeamCymru_"+tc.name, func(t *testing.T) {
			// Directly test Team Cymru DNS API
			lookup, err := asn.FetchASLookupTeamCymru(tc.ip)

			if tc.expectError {
				assert.Error(t, err, "FetchASLookupTeamCymru expected error for %s", tc.ip)
				return
			}

			if err != nil {
				// DNS lookups can fail for various reasons
				if strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "timeout") {
					t.Skipf("Team Cymru DNS unavailable for %s: %v", tc.ip, err)
				}
				require.NoError(t, err, "FetchASLookupTeamCymru returned unexpected error for %s", tc.ip)
			}

			require.NotNil(t, lookup, "FetchASLookupTeamCymru returned nil for %s", tc.ip)

			if tc.expectASN {
				assert.NotEmpty(t, lookup.Asn, "FetchASLookupTeamCymru returned empty ASN for %s", tc.ip)
			}

			if tc.expectName {
				assert.NotEmpty(t, lookup.AsnName, "FetchASLookupTeamCymru returned empty ASN name for %s", tc.ip)
			}

			// Verify IP matches
			assert.Equal(t, tc.ip, lookup.IP, "FetchASLookupTeamCymru returned different IP")

			t.Logf("Team Cymru lookup for %s: ASN=%s, Name=%s, Country=%s, Range=%s",
				tc.ip, lookup.Asn, lookup.AsnName, lookup.Country, lookup.AsnRange)
		})
	}
}

func TestCompareHackerTargetAndTeamCymru(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ASN integration test in short mode")
	}
	if os.Getenv("RUN_ASN_INTEGRATION") == "" {
		t.Skip("set RUN_ASN_INTEGRATION=1 to run ASN integration tests")
	}

	// Test with public IP and known IPs to compare results
	testIPs := []string{}

	// Add current public IP if available
	if publicIP, err := netutil.PublicIP(); err == nil {
		testIPs = append(testIPs, publicIP)
	}

	// Add well-known IPs for comparison
	testIPs = append(testIPs,
		"8.8.8.8",      // Google DNS
		"1.1.1.1",      // Cloudflare DNS
		"52.94.76.131", // AWS IP
		"13.107.42.14", // Azure IP
		"140.82.114.4", // GitHub IP
	)

	for _, ip := range testIPs {
		t.Run("Compare_"+ip, func(t *testing.T) {
			// Fetch from HackerTarget
			hackerTargetLookup, hackerTargetErr := asn.FetchASLookupHackerTarget(ip)

			// Fetch from Team Cymru
			teamCymruLookup, teamCymruErr := asn.FetchASLookupTeamCymru(ip)

			// Skip if either service is unavailable
			if hackerTargetErr != nil && (strings.Contains(hackerTargetErr.Error(), "timeout") || strings.Contains(hackerTargetErr.Error(), "rate limit")) {
				t.Skipf("HackerTarget unavailable for %s: %v", ip, hackerTargetErr)
			}
			if teamCymruErr != nil && (strings.Contains(teamCymruErr.Error(), "no such host") || strings.Contains(teamCymruErr.Error(), "timeout")) {
				t.Skipf("Team Cymru unavailable for %s: %v", ip, teamCymruErr)
			}

			// Both should succeed for valid IPs
			require.NoError(t, hackerTargetErr, "HackerTarget failed for %s", ip)
			require.NoError(t, teamCymruErr, "Team Cymru failed for %s", ip)
			require.NotNil(t, hackerTargetLookup, "HackerTarget returned nil for %s", ip)
			require.NotNil(t, teamCymruLookup, "Team Cymru returned nil for %s", ip)

			// Compare ASN numbers - these should match
			assert.Equal(t, hackerTargetLookup.Asn, teamCymruLookup.Asn,
				"ASN mismatch for %s: HackerTarget=%s, TeamCymru=%s",
				ip, hackerTargetLookup.Asn, teamCymruLookup.Asn)

			// Compare IP ranges - might be formatted differently but should be equivalent
			t.Logf("IP Range comparison for %s: HackerTarget=%s, TeamCymru=%s",
				ip, hackerTargetLookup.AsnRange, teamCymruLookup.AsnRange)

			// ASN names might differ in formatting - normalize and compare
			normalizedHackerTarget := asn.NormalizeASNName(hackerTargetLookup.AsnName)
			normalizedTeamCymru := asn.NormalizeASNName(teamCymruLookup.AsnName)

			t.Logf("Comparison for IP %s:", ip)
			t.Logf("  HackerTarget: ASN=%s, Name='%s' (normalized: '%s'), Country=%s, Range=%s",
				hackerTargetLookup.Asn, hackerTargetLookup.AsnName, normalizedHackerTarget,
				hackerTargetLookup.Country, hackerTargetLookup.AsnRange)
			t.Logf("  Team Cymru:   ASN=%s, Name='%s' (normalized: '%s'), Country=%s, Range=%s",
				teamCymruLookup.Asn, teamCymruLookup.AsnName, normalizedTeamCymru,
				teamCymruLookup.Country, teamCymruLookup.AsnRange)

			// Check if normalized names identify the same provider
			if normalizedHackerTarget != "" && normalizedTeamCymru != "" {
				// Both might normalize to known providers (aws, gcp, azure, etc)
				if isKnownProvider(normalizedHackerTarget) && isKnownProvider(normalizedTeamCymru) {
					assert.Equal(t, normalizedHackerTarget, normalizedTeamCymru,
						"Normalized provider mismatch for %s", ip)
				}
			}
		})
	}
}

func TestFallbackBehavior(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ASN integration test in short mode")
	}
	if os.Getenv("RUN_ASN_INTEGRATION") == "" {
		t.Skip("set RUN_ASN_INTEGRATION=1 to run ASN integration tests")
	}

	// Test that fallback works correctly with a known IP
	// This test verifies the fallback logic by using GetASLookup
	testIP := "8.8.8.8" // Google DNS

	lookup, err := asn.GetASLookup(testIP)
	require.NoError(t, err, "GetASLookup failed for %s", testIP)
	require.NotNil(t, lookup, "GetASLookup returned nil for %s", testIP)

	// Verify we got valid data from either primary or fallback
	assert.NotEmpty(t, lookup.Asn, "GetASLookup returned empty ASN for %s", testIP)
	assert.NotEmpty(t, lookup.AsnName, "GetASLookup returned empty ASN name for %s", testIP)

	t.Logf("Fallback test successful for %s: ASN=%s, Name=%s",
		testIP, lookup.Asn, lookup.AsnName)
}

func TestIPv6Support(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ASN integration test in short mode")
	}
	if os.Getenv("RUN_ASN_INTEGRATION") == "" {
		t.Skip("set RUN_ASN_INTEGRATION=1 to run ASN integration tests")
	}

	// Test with Google's IPv6 DNS
	ipv6 := "2001:4860:4860::8888"

	t.Run("TeamCymru_IPv6", func(t *testing.T) {
		lookup, err := asn.FetchASLookupTeamCymru(ipv6)
		if err != nil {
			// IPv6 DNS lookups may not be available everywhere
			if strings.Contains(err.Error(), "no such host") {
				t.Skipf("IPv6 Team Cymru lookup not available: %v", err)
			}
			require.NoError(t, err, "FetchASLookupTeamCymru failed for IPv6 %s", ipv6)
		}

		require.NotNil(t, lookup, "FetchASLookupTeamCymru returned nil for IPv6")
		assert.NotEmpty(t, lookup.Asn, "FetchASLookupTeamCymru returned empty ASN for IPv6")

		t.Logf("IPv6 Team Cymru lookup for %s: ASN=%s, Name=%s",
			ipv6, lookup.Asn, lookup.AsnName)
	})

	t.Run("HackerTarget_IPv6", func(t *testing.T) {
		lookup, err := asn.FetchASLookupHackerTarget(ipv6)
		if err != nil {
			if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "rate limit") {
				t.Skipf("HackerTarget unavailable for IPv6: %v", err)
			}
		}

		// HackerTarget might not support IPv6, so we don't require success
		if err == nil && lookup != nil {
			assert.NotEmpty(t, lookup.Asn, "HackerTarget returned empty ASN for IPv6")
			t.Logf("IPv6 HackerTarget lookup for %s: ASN=%s, Name=%s",
				ipv6, lookup.Asn, lookup.AsnName)
		} else {
			t.Logf("HackerTarget IPv6 support status: %v", err)
		}
	})
}

// Helper function to check if a normalized ASN name is a known provider
func isKnownProvider(name string) bool {
	knownProviders := []string{"aws", "azure", "gcp", "oci", "yotta", "nebius", "hetzner"}
	for _, provider := range knownProviders {
		if name == provider {
			return true
		}
	}
	return false
}
