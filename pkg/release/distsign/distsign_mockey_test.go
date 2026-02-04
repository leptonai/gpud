package distsign

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewClient_Success tests successful client creation.
// NOTE: roots() uses sync.OnceValue and cannot be mocked directly.
// This test just verifies the actual implementation works.
func TestNewClient_Success(t *testing.T) {
	client, err := NewClient(nil, "https://pkg.gpud.dev")
	require.NoError(t, err)
	assert.NotNil(t, client)
}

// TestNewClient_WithLogf tests client creation with custom logger.
func TestNewClient_WithLogf(t *testing.T) {
	logMessages := []string{}
	logf := func(m string, args ...any) {
		logMessages = append(logMessages, m)
	}

	client, err := NewClient(logf, "https://pkg.gpud.dev")
	require.NoError(t, err)
	assert.NotNil(t, client)
}

// TestNewClient_InvalidURL tests client creation with invalid URL.
func TestNewClient_InvalidURL(t *testing.T) {
	client, err := NewClient(nil, "://invalid-url")
	require.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "invalid pkgsAddr")
}

// TestNewClient_EmptyURL tests client creation with empty URL.
func TestNewClient_EmptyURL(t *testing.T) {
	// Empty URL should still be valid (parsed as relative URL)
	client, err := NewClient(nil, "")
	require.NoError(t, err)
	assert.NotNil(t, client)
}

// TestFetch_Success tests successful fetch.
func TestFetch_Success(t *testing.T) {
	mockey.PatchConvey("fetch success", t, func() {
		expectedContent := "test content"
		mockey.Mock(http.Get).To(func(url string) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(expectedContent)),
			}, nil
		}).Build()

		content, err := Fetch("https://example.com/file", 1024)
		require.NoError(t, err)
		assert.Equal(t, expectedContent, string(content))
	})
}

// TestFetch_HTTPError tests fetch when HTTP request fails.
func TestFetch_HTTPError(t *testing.T) {
	mockey.PatchConvey("fetch http error", t, func() {
		mockey.Mock(http.Get).To(func(url string) (*http.Response, error) {
			return nil, errors.New("connection refused")
		}).Build()

		content, err := Fetch("https://example.com/file", 1024)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection refused")
		assert.Nil(t, content)
	})
}

// TestFetch_RespectsSizeLimit tests that fetch respects size limit.
func TestFetch_RespectsSizeLimit(t *testing.T) {
	mockey.PatchConvey("fetch respects size limit", t, func() {
		// Create content larger than limit
		largeContent := strings.Repeat("x", 1000)
		mockey.Mock(http.Get).To(func(url string) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(largeContent)),
			}, nil
		}).Build()

		content, err := Fetch("https://example.com/file", 100)
		require.NoError(t, err)
		// Content should be limited to 100 bytes
		assert.Equal(t, 100, len(content))
	})
}

// TestClientURL tests the client URL method.
func TestClientURL(t *testing.T) {
	client, err := NewClient(nil, "https://pkg.gpud.dev")
	require.NoError(t, err)

	result := client.url("test/path")
	assert.Equal(t, "https://pkg.gpud.dev/test/path", result)
}

// TestClientURL_WithTrailingSlash tests the client URL method with trailing slash.
func TestClientURL_WithTrailingSlash(t *testing.T) {
	client, err := NewClient(nil, "https://pkg.gpud.dev/")
	require.NoError(t, err)

	result := client.url("test/path")
	assert.Equal(t, "https://pkg.gpud.dev/test/path", result)
}

// TestVerifyAny_NoKeys tests VerifyAny with no keys.
func TestVerifyAny_NoKeys(t *testing.T) {
	msg := []byte("test message")
	sig := []byte("fake signature")

	result := VerifyAny(nil, msg, sig)
	assert.False(t, result)
}

// TestVerifyAny_EmptyKeys tests VerifyAny with empty keys slice.
func TestVerifyAny_EmptyKeys(t *testing.T) {
	msg := []byte("test message")
	sig := []byte("fake signature")

	result := VerifyAny([]ed25519.PublicKey{}, msg, sig)
	assert.False(t, result)
}

// TestVerifyAny_ValidSignature tests VerifyAny with valid signature.
func TestVerifyAny_ValidSignature(t *testing.T) {
	// Generate a key pair
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	msg := []byte("test message")
	sig := ed25519.Sign(priv, msg)

	result := VerifyAny([]ed25519.PublicKey{pub}, msg, sig)
	assert.True(t, result)
}

// TestVerifyAny_InvalidSignature tests VerifyAny with invalid signature.
func TestVerifyAny_InvalidSignature(t *testing.T) {
	// Generate two different key pairs
	pub1, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	_, priv2, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	msg := []byte("test message")
	// Sign with priv2 but verify with pub1
	sig := ed25519.Sign(priv2, msg)

	result := VerifyAny([]ed25519.PublicKey{pub1}, msg, sig)
	assert.False(t, result)
}

// TestVerifyAny_MultipleKeys tests VerifyAny with multiple keys where one is valid.
func TestVerifyAny_MultipleKeys(t *testing.T) {
	// Generate multiple key pairs
	pub1, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pub2, priv2, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pub3, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	msg := []byte("test message")
	sig := ed25519.Sign(priv2, msg)

	// pub2 should verify
	result := VerifyAny([]ed25519.PublicKey{pub1, pub2, pub3}, msg, sig)
	assert.True(t, result)
}

// TestParseSigningKeyBundle_EmptyBundle tests parsing empty bundle.
func TestParseSigningKeyBundle_EmptyBundle(t *testing.T) {
	keys, err := ParseSigningKeyBundle([]byte(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no signing keys found")
	assert.Nil(t, keys)
}

// TestParseSigningKeyBundle_InvalidPEM tests parsing invalid PEM.
func TestParseSigningKeyBundle_InvalidPEM(t *testing.T) {
	keys, err := ParseSigningKeyBundle([]byte("not a pem"))
	require.Error(t, err)
	assert.Nil(t, keys)
}

// TestParseRootKeyBundle_EmptyBundle tests parsing empty bundle.
func TestParseRootKeyBundle_EmptyBundle(t *testing.T) {
	keys, err := ParseRootKeyBundle([]byte(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no signing keys found")
	assert.Nil(t, keys)
}

// TestParseRootKeyBundle_InvalidPEM tests parsing invalid PEM.
func TestParseRootKeyBundle_InvalidPEM(t *testing.T) {
	keys, err := ParseRootKeyBundle([]byte("not a pem"))
	require.Error(t, err)
	assert.Nil(t, keys)
}

// TestGenerateRootKey_Success tests successful root key generation.
func TestGenerateRootKey_Success(t *testing.T) {
	priv, pub, err := GenerateRootKey()
	require.NoError(t, err)
	assert.NotNil(t, priv)
	assert.NotNil(t, pub)
	assert.Contains(t, string(priv), "ROOT PRIVATE KEY")
	assert.Contains(t, string(pub), "ROOT PUBLIC KEY")
}

// TestGenerateSigningKey_Success tests successful signing key generation.
func TestGenerateSigningKey_Success(t *testing.T) {
	priv, pub, err := GenerateSigningKey()
	require.NoError(t, err)
	assert.NotNil(t, priv)
	assert.NotNil(t, pub)
	assert.Contains(t, string(priv), "SIGNING PRIVATE KEY")
	assert.Contains(t, string(pub), "SIGNING PUBLIC KEY")
}

// TestParseRootKey_Success tests successful root key parsing.
func TestParseRootKey_Success(t *testing.T) {
	priv, _, err := GenerateRootKey()
	require.NoError(t, err)

	key, err := ParseRootKey(priv)
	require.NoError(t, err)
	assert.NotNil(t, key)
}

// TestParseRootKey_WrongKeyType tests parsing signing key as root key.
func TestParseRootKey_WrongKeyType(t *testing.T) {
	priv, _, err := GenerateSigningKey()
	require.NoError(t, err)

	key, err := ParseRootKey(priv)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse root key")
	assert.Nil(t, key)
}

// TestParseSigningKey_Success tests successful signing key parsing.
func TestParseSigningKey_Success(t *testing.T) {
	priv, _, err := GenerateSigningKey()
	require.NoError(t, err)

	key, err := ParseSigningKey(priv)
	require.NoError(t, err)
	assert.NotNil(t, key)
}

// TestParseSigningKey_WrongKeyType tests parsing root key as signing key.
func TestParseSigningKey_WrongKeyType(t *testing.T) {
	priv, _, err := GenerateRootKey()
	require.NoError(t, err)

	key, err := ParseSigningKey(priv)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse root key")
	assert.Nil(t, key)
}

// TestRootKeySignSigningKeys tests root key signing of signing keys.
func TestRootKeySignSigningKeys(t *testing.T) {
	// Generate root key
	rootPriv, _, err := GenerateRootKey()
	require.NoError(t, err)
	rootKey, err := ParseRootKey(rootPriv)
	require.NoError(t, err)

	// Generate signing key
	_, signingPub, err := GenerateSigningKey()
	require.NoError(t, err)

	// Sign the signing key bundle
	sig, err := rootKey.SignSigningKeys(signingPub)
	require.NoError(t, err)
	assert.NotNil(t, sig)
	assert.Equal(t, ed25519.SignatureSize, len(sig))
}

// TestRootKeySignSigningKeys_InvalidBundle tests signing invalid bundle.
func TestRootKeySignSigningKeys_InvalidBundle(t *testing.T) {
	rootPriv, _, err := GenerateRootKey()
	require.NoError(t, err)
	rootKey, err := ParseRootKey(rootPriv)
	require.NoError(t, err)

	sig, err := rootKey.SignSigningKeys([]byte("invalid bundle"))
	require.Error(t, err)
	assert.Nil(t, sig)
}

// TestSigningKeySignPackageHash_Success tests successful package hash signing.
func TestSigningKeySignPackageHash_Success(t *testing.T) {
	signingPriv, _, err := GenerateSigningKey()
	require.NoError(t, err)
	signingKey, err := ParseSigningKey(signingPriv)
	require.NoError(t, err)

	hash := make([]byte, 32) // 256-bit hash
	for i := range hash {
		hash[i] = byte(i)
	}

	sig, err := signingKey.SignPackageHash(hash, 1024)
	require.NoError(t, err)
	assert.NotNil(t, sig)
	assert.Equal(t, ed25519.SignatureSize, len(sig))
}

// TestSigningKeySignPackageHash_ZeroLength tests signing with zero length.
func TestSigningKeySignPackageHash_ZeroLength(t *testing.T) {
	signingPriv, _, err := GenerateSigningKey()
	require.NoError(t, err)
	signingKey, err := ParseSigningKey(signingPriv)
	require.NoError(t, err)

	hash := make([]byte, 32)
	sig, err := signingKey.SignPackageHash(hash, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package length must be positive")
	assert.Nil(t, sig)
}

// TestSigningKeySignPackageHash_NegativeLength tests signing with negative length.
func TestSigningKeySignPackageHash_NegativeLength(t *testing.T) {
	signingPriv, _, err := GenerateSigningKey()
	require.NoError(t, err)
	signingKey, err := ParseSigningKey(signingPriv)
	require.NoError(t, err)

	hash := make([]byte, 32)
	sig, err := signingKey.SignPackageHash(hash, -1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package length must be positive")
	assert.Nil(t, sig)
}

// TestNewPackageHash tests PackageHash creation and operations.
func TestNewPackageHash(t *testing.T) {
	ph := NewPackageHash()
	require.NotNil(t, ph)

	// Initial state
	assert.Equal(t, int64(0), ph.Len())

	// Write some data
	n, err := ph.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, int64(5), ph.Len())

	// Write more data
	n, err = ph.Write([]byte(" world"))
	require.NoError(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, int64(11), ph.Len())

	// Get hash
	sum := ph.Sum(nil)
	assert.Equal(t, 32, len(sum)) // BLAKE2s produces 256-bit hash

	// Reset
	ph.Reset()
	assert.Equal(t, int64(0), ph.Len())
}

// TestClientDownload_SigningKeysError tests download when signing keys fetch fails.
func TestClientDownload_SigningKeysError(t *testing.T) {
	mockey.PatchConvey("signing keys error", t, func() {
		u, _ := url.Parse("https://example.com")
		client := &Client{
			logf:     func(string, ...any) {},
			roots:    []ed25519.PublicKey{make([]byte, ed25519.PublicKeySize)},
			pkgsAddr: u,
		}

		mockey.Mock(Fetch).To(func(url string, limit int64) ([]byte, error) {
			return nil, errors.New("failed to fetch signing keys")
		}).Build()

		tempDir := t.TempDir()
		dstPath := filepath.Join(tempDir, "test.bin")

		err := client.Download(context.Background(), "test.bin", dstPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch signing keys")
	})
}

// TestClientValidateLocalBinary_FileNotFound tests validation when file doesn't exist.
func TestClientValidateLocalBinary_FileNotFound(t *testing.T) {
	mockey.PatchConvey("file not found", t, func() {
		u, _ := url.Parse("https://example.com")
		client := &Client{
			logf:     func(string, ...any) {},
			roots:    []ed25519.PublicKey{make([]byte, ed25519.PublicKeySize)},
			pkgsAddr: u,
		}

		mockey.Mock(Fetch).To(func(url string, limit int64) ([]byte, error) {
			return []byte(""), nil
		}).Build()

		mockey.Mock((*Client).signingKeys).To(func(c *Client) ([]ed25519.PublicKey, error) {
			return nil, errors.New("failed to get signing keys")
		}).Build()

		err := client.ValidateLocalBinary("test.bin", "/nonexistent/file.bin")
		require.Error(t, err)
	})
}

// TestClientValidateLocalBinary_SigningKeysError tests validation when signing keys fetch fails.
func TestClientValidateLocalBinary_SigningKeysError(t *testing.T) {
	mockey.PatchConvey("signing keys error", t, func() {
		u, _ := url.Parse("https://example.com")
		client := &Client{
			logf:     func(string, ...any) {},
			roots:    []ed25519.PublicKey{make([]byte, ed25519.PublicKeySize)},
			pkgsAddr: u,
		}

		mockey.Mock(Fetch).To(func(url string, limit int64) ([]byte, error) {
			return nil, errors.New("network error")
		}).Build()

		tempDir := t.TempDir()
		localFile := filepath.Join(tempDir, "test.bin")
		err := os.WriteFile(localFile, []byte("test content"), 0644)
		require.NoError(t, err)

		err = client.ValidateLocalBinary("test.bin", localFile)
		require.Error(t, err)
	})
}
