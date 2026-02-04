package providers

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderTable(t *testing.T) {
	tests := []struct {
		name          string
		info          Info
		expectedLines []string // Lines that should appear in the output
	}{
		{
			name: "complete info",
			info: Info{
				Provider:      "aws",
				PublicIP:      "203.0.113.1",
				PrivateIP:     "10.0.1.5",
				VMEnvironment: "production",
				InstanceID:    "i-0123456789abcdef",
			},
			expectedLines: []string{
				"Provider",
				"aws",
				"Public IP",
				"203.0.113.1",
				"Private IP",
				"10.0.1.5",
				"VM Environment",
				"production",
				"Instance ID",
				"i-0123456789abcdef",
			},
		},
		{
			name: "partial info - only provider and public IP",
			info: Info{
				Provider: "azure",
				PublicIP: "198.51.100.1",
			},
			expectedLines: []string{
				"Provider",
				"azure",
				"Public IP",
				"198.51.100.1",
			},
		},
		{
			name: "empty info",
			info: Info{},
			expectedLines: []string{
				"Provider",
				"Public IP",
				"Private IP",
			},
		},
		{
			name: "GCP info",
			info: Info{
				Provider:      "gcp",
				PublicIP:      "35.1.1.1",
				PrivateIP:     "10.128.0.2",
				VMEnvironment: "us-central1-a",
				InstanceID:    "1234567890123456789",
			},
			expectedLines: []string{
				"gcp",
				"35.1.1.1",
				"10.128.0.2",
				"us-central1-a",
				"1234567890123456789",
			},
		},
		{
			name: "special characters in values",
			info: Info{
				Provider:   "test-provider",
				PublicIP:   "1.2.3.4",
				InstanceID: "inst-abc_123-xyz",
			},
			expectedLines: []string{
				"test-provider",
				"1.2.3.4",
				"inst-abc_123-xyz",
			},
		},
		{
			name: "empty strings",
			info: Info{
				Provider:      "",
				PublicIP:      "",
				PrivateIP:     "",
				VMEnvironment: "",
				InstanceID:    "",
			},
			expectedLines: []string{
				"Provider",
				"Public IP",
				"Private IP",
				"VM Environment",
				"Instance ID",
			},
		},
		{
			name: "IPv6 addresses",
			info: Info{
				Provider:  "aws",
				PublicIP:  "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
				PrivateIP: "fd00::1",
			},
			expectedLines: []string{
				"2001:0db8:85a3:0000:0000:8a2e:0370:7334",
				"fd00::1",
			},
		},
		{
			name: "long values",
			info: Info{
				Provider:   "very-long-provider-name-that-exceeds-normal-length",
				InstanceID: "i-" + strings.Repeat("a", 50),
			},
			expectedLines: []string{
				"very-long-provider-name-that-exceeds-normal-length",
				"i-" + strings.Repeat("a", 50),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			tc.info.RenderTable(&buf)
			result := buf.String()

			// Verify the result is not empty
			assert.NotEmpty(t, result, "RenderTable should return non-empty string")

			// Verify expected lines appear in the output
			for _, expectedLine := range tc.expectedLines {
				assert.Contains(t, result, expectedLine,
					"Output should contain '%s'", expectedLine)
			}

			// Verify it looks like a table (contains common table characters)
			// The tablewriter package typically uses box-drawing characters or pipes
			hasTableFormat := strings.Contains(result, "|") ||
				strings.Contains(result, "+") ||
				strings.Contains(result, "-")
			assert.True(t, hasTableFormat, "Output should be formatted as a table")
		})
	}
}

func TestRenderTable_OutputStructure(t *testing.T) {
	info := Info{
		Provider:      "aws",
		PublicIP:      "1.2.3.4",
		PrivateIP:     "10.0.0.1",
		VMEnvironment: "prod",
		InstanceID:    "i-abc123",
	}

	var buf bytes.Buffer
	info.RenderTable(&buf)
	result := buf.String()

	// Check that output has multiple lines
	lines := strings.Split(result, "\n")
	assert.Greater(t, len(lines), 1, "Table should have multiple lines")

	// Check that the output is not just whitespace
	trimmed := strings.TrimSpace(result)
	assert.NotEmpty(t, trimmed, "Table should have visible content")
}

func TestRenderTable_NilInfo(t *testing.T) {
	var info *Info
	var buf bytes.Buffer
	info.RenderTable(&buf)
	result := buf.String()

	// Nil info should not write anything
	assert.Empty(t, result, "Nil info should produce empty output")
}
