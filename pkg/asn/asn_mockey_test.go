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

func TestFetchASLookupHackerTarget_WithMockey(t *testing.T) {
	mockey.PatchConvey("fetchASLookupHackerTarget parses response", t, func() {
		mockey.Mock(http.Get).To(func(url string) (*http.Response, error) {
			body := `{"asn":"15169","asn_name":"GOOGLE, US","asn_range":"15169-15169","country":"US","ip":"8.8.8.8"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}).Build()

		resp, err := fetchASLookupHackerTarget("8.8.8.8")
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "google", resp.AsnName)
		assert.Equal(t, "us", resp.Country)
		assert.Equal(t, "15169", resp.Asn)
	})
}

func TestFetchASLookupTeamCymru_WithMockey(t *testing.T) {
	mockey.PatchConvey("fetchASLookupTeamCymru parses DNS records", t, func() {
		mockey.Mock(net.LookupTXT).To(func(name string) ([]string, error) {
			if strings.HasSuffix(name, ".origin.asn.cymru.com") {
				return []string{"15169 | 8.8.8.0/24 | US | arin | 2000-03-30"}, nil
			}
			if strings.HasPrefix(name, "AS15169.") {
				return []string{"15169 | US | arin | 2000-03-30 | GOOGLE, US"}, nil
			}
			return nil, errors.New("unexpected query")
		}).Build()

		resp, err := fetchASLookupTeamCymru("8.8.8.8")
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "15169", resp.Asn)
		assert.Equal(t, "8.8.8.0/24", resp.AsnRange)
		assert.Equal(t, "google", resp.AsnName)
		assert.Equal(t, "us", resp.Country)
		assert.Equal(t, "8.8.8.8", resp.IP)
	})
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

func TestFetchASLookupTeamCymru_ErrorBranchesWithMockey(t *testing.T) {
	mockey.PatchConvey("fetchASLookupTeamCymru no origin records", t, func() {
		mockey.Mock(net.LookupTXT).To(func(name string) ([]string, error) {
			return []string{}, nil
		}).Build()

		_, err := fetchASLookupTeamCymru("8.8.8.8")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no TXT records")
	})

	mockey.PatchConvey("fetchASLookupTeamCymru invalid origin format", t, func() {
		mockey.Mock(net.LookupTXT).To(func(name string) ([]string, error) {
			if strings.HasSuffix(name, ".origin.asn.cymru.com") {
				return []string{"invalid"}, nil
			}
			return nil, errors.New("unexpected")
		}).Build()

		_, err := fetchASLookupTeamCymru("8.8.8.8")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected response format")
	})

	mockey.PatchConvey("fetchASLookupTeamCymru no details records", t, func() {
		mockey.Mock(net.LookupTXT).To(func(name string) ([]string, error) {
			if strings.HasSuffix(name, ".origin.asn.cymru.com") {
				return []string{"15169 | 8.8.8.0/24 | US | arin | 2000-03-30"}, nil
			}
			return []string{}, nil
		}).Build()

		_, err := fetchASLookupTeamCymru("8.8.8.8")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no TXT records")
	})

	mockey.PatchConvey("fetchASLookupTeamCymru invalid details format", t, func() {
		mockey.Mock(net.LookupTXT).To(func(name string) ([]string, error) {
			if strings.HasSuffix(name, ".origin.asn.cymru.com") {
				return []string{"15169 | 8.8.8.0/24 | US | arin | 2000-03-30"}, nil
			}
			return []string{"bad"}, nil
		}).Build()

		_, err := fetchASLookupTeamCymru("8.8.8.8")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected response format")
	})
}
