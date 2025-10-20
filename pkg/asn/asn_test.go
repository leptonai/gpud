package asn

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withLookupMocks(primary func(string) (*ASLookupResponse, error), fallback func(string) (*ASLookupResponse, error)) func() {
	origPrimary := lookupPrimary
	origFallback := lookupFallback
	lookupPrimary = primary
	lookupFallback = fallback
	return func() {
		lookupPrimary = origPrimary
		lookupFallback = origFallback
	}
}

func TestGetASLookupPrimarySuccess(t *testing.T) {
	restore := withLookupMocks(
		func(ip string) (*ASLookupResponse, error) {
			return &ASLookupResponse{Asn: "123", AsnName: "primary", IP: ip}, nil
		},
		func(ip string) (*ASLookupResponse, error) {
			t.Fatalf("fallback should not be called when primary succeeds")
			return nil, nil
		},
	)
	defer restore()

	resp, err := GetASLookup("1.1.1.1")
	require.NoError(t, err)
	assert.Equal(t, "primary", resp.AsnName)
}

func TestGetASLookupFallbackOnEmptyName(t *testing.T) {
	fallbackCalled := false
	restore := withLookupMocks(
		func(ip string) (*ASLookupResponse, error) {
			return &ASLookupResponse{Asn: "123", AsnName: ""}, nil
		},
		func(ip string) (*ASLookupResponse, error) {
			fallbackCalled = true
			return &ASLookupResponse{Asn: "456", AsnName: "fallback"}, nil
		},
	)
	defer restore()

	resp, err := GetASLookup("1.1.1.1")
	require.NoError(t, err)
	assert.True(t, fallbackCalled, "expected fallback to be called")
	assert.Equal(t, "fallback", resp.AsnName)
}

func TestGetASLookupFallbackOnPrimaryError(t *testing.T) {
	fallbackCalled := false
	restore := withLookupMocks(
		func(ip string) (*ASLookupResponse, error) {
			return nil, errors.New("boom")
		},
		func(ip string) (*ASLookupResponse, error) {
			fallbackCalled = true
			return &ASLookupResponse{Asn: "456", AsnName: "fallback"}, nil
		},
	)
	defer restore()

	resp, err := GetASLookup("2.2.2.2")
	require.NoError(t, err)
	assert.True(t, fallbackCalled, "expected fallback to be called")
	assert.Equal(t, "fallback", resp.AsnName)
}

func TestGetASLookupBothFail(t *testing.T) {
	restore := withLookupMocks(
		func(ip string) (*ASLookupResponse, error) {
			return nil, errors.New("primary")
		},
		func(ip string) (*ASLookupResponse, error) {
			return nil, errors.New("fallback")
		},
	)
	defer restore()

	resp, err := GetASLookup("3.3.3.3")
	require.Error(t, err, "expected error when both lookups fail")
	assert.Nil(t, resp, "expected nil response when both lookups fail")
}

func TestSanitizeASNNameAndCountry(t *testing.T) {
	tests := []struct {
		name            string
		inputName       string
		fallbackCountry string
		expectName      string
		expectCountry   string
	}{
		{
			name:            "name with country suffix",
			inputName:       "nvidia-net, us",
			fallbackCountry: "",
			expectName:      "nvidia-net",
			expectCountry:   "us",
		},
		{
			name:            "fallback country used",
			inputName:       "aws",
			fallbackCountry: "US",
			expectName:      "aws",
			expectCountry:   "us",
		},
		{
			name:            "empty name",
			inputName:       "",
			fallbackCountry: "CA",
			expectName:      "",
			expectCountry:   "ca",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name, country := sanitizeASNNameAndCountry(tc.inputName, tc.fallbackCountry)
			assert.Equal(t, tc.expectName, name)
			assert.Equal(t, tc.expectCountry, country)
		})
	}
}

func TestNormalizeASNName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Test exact keyword matches
		{
			name:     "exact aws match",
			input:    "aws",
			expected: "aws",
		},
		{
			name:     "exact azure match",
			input:    "azure",
			expected: "azure",
		},
		{
			name:     "exact gcp match",
			input:    "gcp",
			expected: "gcp",
		},
		{
			name:     "exact google match",
			input:    "google",
			expected: "gcp",
		},
		{
			name:     "exact yotta match",
			input:    "yotta",
			expected: "yotta",
		},

		// Test case insensitive matching
		{
			name:     "uppercase AWS",
			input:    "AWS",
			expected: "aws",
		},
		{
			name:     "mixed case Azure",
			input:    "Azure",
			expected: "azure",
		},
		{
			name:     "uppercase GOOGLE",
			input:    "GOOGLE",
			expected: "gcp",
		},

		// Test keywords contained in larger strings
		{
			name:     "aws in company name",
			input:    "amazon aws services",
			expected: "aws",
		},
		{
			name:     "azure in company name",
			input:    "microsoft azure cloud",
			expected: "azure",
		},
		{
			name:     "google in company name",
			input:    "google cloud platform",
			expected: "gcp",
		},
		{
			name:     "gcp in company name",
			input:    "gcp infrastructure",
			expected: "gcp",
		},
		{
			name:     "yotta in company name",
			input:    "yotta infrastructure",
			expected: "yotta",
		},

		// Test whitespace handling
		{
			name:     "leading whitespace",
			input:    "  aws",
			expected: "aws",
		},
		{
			name:     "trailing whitespace",
			input:    "azure  ",
			expected: "azure",
		},
		{
			name:     "both leading and trailing whitespace",
			input:    "  google  ",
			expected: "gcp",
		},
		{
			name:     "whitespace with company name",
			input:    "  amazon aws services  ",
			expected: "aws",
		},

		// Test non-matching cases
		{
			name:     "unknown provider",
			input:    "digitalocean",
			expected: "digitalocean",
		},
		{
			name:     "random string",
			input:    "some random provider",
			expected: "some random provider",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},

		// Test substring matches (function uses strings.Contains, so these will match)
		{
			name:     "aws as substring",
			input:    "awsome provider",
			expected: "aws", // This will match because "awsome" contains "aws"
		},
		{
			name:     "azure as substring",
			input:    "lazure provider",
			expected: "azure", // This will match because "lazure" contains "azure"
		},
		{
			name:     "gcp as substring",
			input:    "pgcp provider",
			expected: "gcp", // This will match because "pgcp" contains "gcp"
		},

		// Test mixed case with complex strings
		{
			name:     "mixed case complex string",
			input:    "AMAZON Web Services AWS",
			expected: "aws",
		},
		{
			name:     "mixed case with google",
			input:    "Google Cloud Platform GCP",
			expected: "gcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeASNName(tt.input)
			assert.Equal(t, tt.expected, result, "NormalizeASNName(%q) should return %q", tt.input, tt.expected)
		})
	}
}

// TestNormalizeASNNameDeterministic tests that the function is deterministic
// for inputs that don't contain multiple keywords
func TestNormalizeASNNameDeterministic(t *testing.T) {
	testCases := []string{
		"aws provider",
		"azure services",
		"google cloud",
		"yotta infrastructure",
		"unknown provider",
	}

	for _, input := range testCases {
		// Run the same input multiple times to ensure deterministic behavior
		results := make(map[string]int)
		for range 10 {
			result := NormalizeASNName(input)
			results[result]++
		}

		assert.Equal(t, 1, len(results), "NormalizeASNName(%q) produced non-deterministic results: %v", input, results)
	}
}

// TestNormalizeASNNameMultipleKeywords tests behavior when multiple keywords are present
// Note: Due to Go's map iteration order being non-deterministic, this test verifies
// that the result is one of the valid expected values
func TestNormalizeASNNameMultipleKeywords(t *testing.T) {
	testInput := "aws google azure yotta"

	// The result should be one of the expected normalized names
	validResults := map[string]bool{
		"aws":   true,
		"gcp":   true,
		"azure": true,
		"yotta": true,
	}

	result := NormalizeASNName(testInput)
	assert.Contains(t, validResults, result, "NormalizeASNName(%q) should return one of: aws, gcp, azure, yotta", testInput)
}

// TestNormalizeASNNameSpecialCharacters tests the function with special characters
func TestNormalizeASNNameSpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "special characters with aws",
			input:    "aws-services-123",
			expected: "aws",
		},
		{
			name:     "special characters with google",
			input:    "google.cloud",
			expected: "gcp",
		},
		{
			name:     "unicode characters with azure",
			input:    "azure™ services",
			expected: "azure",
		},
		{
			name:     "punctuation only",
			input:    "!@#$%^&*()",
			expected: "!@#$%^&*()",
		},
		{
			name:     "numbers only",
			input:    "12345",
			expected: "12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeASNName(tt.input)
			assert.Equal(t, tt.expected, result, "NormalizeASNName(%q) should return %q", tt.input, tt.expected)
		})
	}
}
