package asn

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withHTTPGetMock replaces httpGetFunc for the duration of the test and restores it on cleanup.
// Not safe for use with t.Parallel().
func withHTTPGetMock(t *testing.T, fn func(url string) (*http.Response, error)) {
	t.Helper()
	orig := httpGetFunc
	httpGetFunc = fn
	t.Cleanup(func() { httpGetFunc = orig })
}

// withDNSLookupTXTMock replaces dnsLookupTXTFunc for the duration of the test and restores it on cleanup.
// Not safe for use with t.Parallel().
func withDNSLookupTXTMock(t *testing.T, fn func(name string) ([]string, error)) {
	t.Helper()
	orig := dnsLookupTXTFunc
	dnsLookupTXTFunc = fn
	t.Cleanup(func() { dnsLookupTXTFunc = orig })
}

func TestGetASLookup_RetriesWithMockey(t *testing.T) {
	mockey.PatchConvey("GetASLookup retries with sleep", t, func() {
		origPrimary := lookupPrimary
		origFallback := lookupFallback
		defer func() {
			lookupPrimary = origPrimary
			lookupFallback = origFallback
		}()

		attempts := 0
		lookupPrimary = func(ip string) (*ASLookupResponse, error) {
			attempts++
			return nil, errors.New("primary down")
		}
		lookupFallback = func(ip string) (*ASLookupResponse, error) {
			return nil, errors.New("fallback down")
		}

		sleepCalls := 0
		mockey.Mock(time.Sleep).To(func(d time.Duration) {
			sleepCalls++
		}).Build()

		resp, err := GetASLookup("1.2.3.4")
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, asLookupMaxRetries, attempts)
		assert.Equal(t, asLookupMaxRetries-1, sleepCalls)
	})
}

func TestFetchASLookupHackerTarget_Success(t *testing.T) {
	withHTTPGetMock(t, func(url string) (*http.Response, error) {
		body := `{"asn":"15169","asn_name":"GOOGLE, US","asn_range":"15169-15169","country":"US","ip":"8.8.8.8"}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	resp, err := fetchASLookupHackerTarget("8.8.8.8")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "google", resp.AsnName)
	assert.Equal(t, "us", resp.Country)
	assert.Equal(t, "15169", resp.Asn)
}

func TestFetchASLookupHackerTarget_HTTPError(t *testing.T) {
	withHTTPGetMock(t, func(url string) (*http.Response, error) {
		return nil, errors.New("connection timeout")
	})

	result, err := fetchASLookupHackerTarget("8.8.8.8")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection timeout")
	assert.Nil(t, result)
}

func TestFetchASLookupHackerTarget_Non200Status(t *testing.T) {
	withHTTPGetMock(t, func(url string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(strings.NewReader("Internal Server Error")),
		}, nil
	})

	result, err := fetchASLookupHackerTarget("8.8.8.8")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid character")
	assert.Nil(t, result)
}

func TestFetchASLookupHackerTarget_InvalidJSON(t *testing.T) {
	withHTTPGetMock(t, func(url string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("invalid format")),
		}, nil
	})

	result, err := fetchASLookupHackerTarget("8.8.8.8")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid character")
	assert.Nil(t, result)
}

func TestFetchASLookupTeamCymru_Success(t *testing.T) {
	withDNSLookupTXTMock(t, func(name string) ([]string, error) {
		if strings.HasSuffix(name, ".origin.asn.cymru.com") {
			return []string{"15169 | 8.8.8.0/24 | US | arin | 2000-03-30"}, nil
		}
		if strings.HasPrefix(name, "AS15169.") {
			return []string{"15169 | US | arin | 2000-03-30 | GOOGLE, US"}, nil
		}
		return nil, errors.New("unexpected query")
	})

	resp, err := fetchASLookupTeamCymru("8.8.8.8")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "15169", resp.Asn)
	assert.Equal(t, "8.8.8.0/24", resp.AsnRange)
	assert.Equal(t, "google", resp.AsnName)
	assert.Equal(t, "us", resp.Country)
	assert.Equal(t, "8.8.8.8", resp.IP)
}

func TestTeamCymruOriginDomainVariants(t *testing.T) {
	ipv4 := net.ParseIP("8.8.8.8")
	origin, err := teamCymruOriginDomain(ipv4)
	require.NoError(t, err)
	assert.Contains(t, origin, "origin.asn.cymru.com")

	ipv6 := net.ParseIP("2001:db8::1")
	origin6, err := teamCymruOriginDomain(ipv6)
	require.NoError(t, err)
	assert.Contains(t, origin6, "origin6.asn.cymru.com")

	_, err = teamCymruOriginDomain(net.IP{})
	require.Error(t, err)
}

func TestOriginFieldsValueOutOfRange(t *testing.T) {
	assert.Equal(t, "", originFieldsValue([]string{"one"}, 2))
}

func TestGetASLookup_EmptyNameRetriesWithMockey(t *testing.T) {
	mockey.PatchConvey("GetASLookup retries when empty name and fallback fails", t, func() {
		origPrimary := lookupPrimary
		origFallback := lookupFallback
		defer func() {
			lookupPrimary = origPrimary
			lookupFallback = origFallback
		}()

		attempts := 0
		lookupPrimary = func(ip string) (*ASLookupResponse, error) {
			attempts++
			return &ASLookupResponse{Asn: "123", AsnName: ""}, nil
		}
		lookupFallback = func(ip string) (*ASLookupResponse, error) {
			return nil, errors.New("fallback failed")
		}

		mockey.Mock(time.Sleep).To(func(time.Duration) {}).Build()

		resp, err := GetASLookup("9.9.9.9")
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.AsnName)
		assert.Equal(t, asLookupMaxRetries, attempts)
	})
}

func TestGetASLookup_PrimaryErrorFallbackEmptyWithMockey(t *testing.T) {
	mockey.PatchConvey("GetASLookup returns fallback even if empty name", t, func() {
		origPrimary := lookupPrimary
		origFallback := lookupFallback
		defer func() {
			lookupPrimary = origPrimary
			lookupFallback = origFallback
		}()

		lookupPrimary = func(ip string) (*ASLookupResponse, error) {
			return nil, errors.New("primary failed")
		}
		lookupFallback = func(ip string) (*ASLookupResponse, error) {
			return &ASLookupResponse{Asn: "456", AsnName: "", IP: ip}, nil
		}

		resp, err := GetASLookup("4.4.4.4")
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.AsnName)
		assert.Equal(t, "456", resp.Asn)
	})
}

func TestFetchASLookupTeamCymru_ErrorBranches(t *testing.T) {
	t.Run("no origin records", func(t *testing.T) {
		withDNSLookupTXTMock(t, func(name string) ([]string, error) {
			return []string{}, nil
		})

		_, err := fetchASLookupTeamCymru("8.8.8.8")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no TXT records")
	})

	t.Run("invalid origin format", func(t *testing.T) {
		withDNSLookupTXTMock(t, func(name string) ([]string, error) {
			if strings.HasSuffix(name, ".origin.asn.cymru.com") {
				return []string{"invalid"}, nil
			}
			return nil, errors.New("unexpected")
		})

		_, err := fetchASLookupTeamCymru("8.8.8.8")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected response format")
	})

	t.Run("no details records", func(t *testing.T) {
		withDNSLookupTXTMock(t, func(name string) ([]string, error) {
			if strings.HasSuffix(name, ".origin.asn.cymru.com") {
				return []string{"15169 | 8.8.8.0/24 | US | arin | 2000-03-30"}, nil
			}
			return []string{}, nil
		})

		_, err := fetchASLookupTeamCymru("8.8.8.8")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no TXT records")
	})

	t.Run("invalid details format", func(t *testing.T) {
		withDNSLookupTXTMock(t, func(name string) ([]string, error) {
			if strings.HasSuffix(name, ".origin.asn.cymru.com") {
				return []string{"15169 | 8.8.8.0/24 | US | arin | 2000-03-30"}, nil
			}
			return []string{"bad"}, nil
		})

		_, err := fetchASLookupTeamCymru("8.8.8.8")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected response format")
	})

	t.Run("DNS lookup error", func(t *testing.T) {
		withDNSLookupTXTMock(t, func(name string) ([]string, error) {
			return nil, errors.New("no such host")
		})

		result, err := fetchASLookupTeamCymru("8.8.8.8")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no such host")
		assert.Nil(t, result)
	})

	t.Run("invalid IP address", func(t *testing.T) {
		result, err := fetchASLookupTeamCymru("not-an-ip")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid IP address")
		assert.Nil(t, result)
	})
}
