package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The destination travels in the request, so the prefix list is the only thing
// standing between a wrong control plane and an arbitrary root-owned write.
func TestValidateNodeCredentialFileRejectsPathsOutsideAllowedTrees(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
	}{
		{"a unit file", "/etc/systemd/system/evil.service"},
		{"an authorized_keys", "/root/.ssh/authorized_keys"},
		{"a shell profile", "/etc/profile.d/evil.sh"},
		{"a relative path", "var/lib/gpud/x"},
		{"traversal out of an allowed tree", "/var/lib/gpud/../../etc/systemd/system/evil.service"},
		{"a prefix that only looks allowed", "/var/lib/gpud-evil/x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNodeCredentialFile(NodeCredentialFile{Path: tc.path, Contents: []byte("x")})
			require.Error(t, err)
		})
	}
}

func TestValidateNodeCredentialFileAcceptsAllowedTrees(t *testing.T) {
	for _, path := range []string{
		"/var/lib/gpud/packages/kubelet/kubelet.yaml",
		"/var/lib/gpud/packages/kubelet/kubelet-client-current.pem",
		"/etc/kubernetes/pki/kubelet-client-current.pem",
	} {
		require.NoError(t, validateNodeCredentialFile(NodeCredentialFile{Path: path, Contents: []byte("x")}), path)
	}
}

// A credential truncated to nothing looks like a successful write to the caller
// and fails wherever the file is read, which is much further from the cause.
func TestValidateNodeCredentialFileRejectsEmptyContents(t *testing.T) {
	err := validateNodeCredentialFile(NodeCredentialFile{Path: "/var/lib/gpud/x", Contents: nil})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no contents")
}

// One bad path must leave the node exactly as it was, not half-updated. The
// good file is listed first so that a per-file validate-then-write loop would
// have created it before reaching the bad one.
func TestProcessNodeCredentialsWritesNothingWhenAnyFileIsRejected(t *testing.T) {
	dir := t.TempDir()
	previous := allowedNodeCredentialPrefixes
	allowedNodeCredentialPrefixes = func() []string { return []string{dir + "/"} }
	t.Cleanup(func() { allowedNodeCredentialPrefixes = previous })

	good := filepath.Join(dir, "good.pem")

	session := &Session{}
	response := &Response{}
	session.processNodeCredentials(context.Background(), Request{
		NodeCredentials: &NodeCredentialsRequest{Kubelet: []NodeCredentialFile{
			{Path: good, Contents: []byte("first")},
			{Path: "/etc/systemd/system/evil.service", Contents: []byte("second")},
		}},
	}, response)

	require.NotEmpty(t, response.Error)
	_, err := os.Stat(good)
	assert.True(t, os.IsNotExist(err), "a rejected request must not write any file")
}

func TestProcessNodeCredentialsRequiresContent(t *testing.T) {
	session := &Session{}

	response := &Response{}
	session.processNodeCredentials(context.Background(), Request{}, response)
	assert.Contains(t, response.Error, "required")

	response = &Response{}
	session.processNodeCredentials(context.Background(), Request{
		NodeCredentials: &NodeCredentialsRequest{},
	}, response)
	assert.Contains(t, response.Error, "no files")
}

// The mode is applied to the staged file before any contents are written, so a
// permissive umask cannot expose a private key even briefly.
func TestWriteNodeCredentialFileNeverExposesContentsUnderPermissiveUmask(t *testing.T) {
	previous := syscall.Umask(0o000)
	defer syscall.Umask(previous)

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "key.pem")
	require.NoError(t, writeNodeCredentialFile(NodeCredentialFile{Path: path, Contents: []byte("secret")}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "default mode must be owner-only")

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "secret", string(contents))
}

func TestWriteNodeCredentialFileHonorsRequestedMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, writeNodeCredentialFile(NodeCredentialFile{
		Path: path, Contents: []byte("providerID: x"), Mode: 0o644,
	}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

// Rewriting a credential must replace it in one step, and must not leave the
// staging file behind for something else to pick up.
func TestWriteNodeCredentialFileReplacesAtomicallyAndLeavesNoResidue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key.pem")

	require.NoError(t, writeNodeCredentialFile(NodeCredentialFile{Path: path, Contents: []byte("first")}))
	require.NoError(t, writeNodeCredentialFile(NodeCredentialFile{Path: path, Contents: []byte("second")}))

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "second", string(contents))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), ".node-credential-"),
			"staging file left behind: %s", entry.Name())
	}
	assert.Len(t, entries, 1)
}

// The audit copy is written to logs, so nothing under node_credentials may
// survive it. Redacting the whole subtree keeps that true as fields are added.
func TestAuditSessionRequestDataRedactsNodeCredentials(t *testing.T) {
	raw, err := json.Marshal(Request{
		Method: "nodeCredentials",
		NodeCredentials: &NodeCredentialsRequest{Kubelet: []NodeCredentialFile{
			{Path: "/var/lib/gpud/packages/kubelet/kubelet-client-current.pem",
				Contents: []byte("BEGIN EC PRIVATE KEY very-secret")},
		}},
	})
	require.NoError(t, err)

	audited := auditSessionRequestData(raw)

	serialized, err := json.Marshal(audited)
	require.NoError(t, err)
	assert.NotContains(t, string(serialized), "very-secret")
	assert.NotContains(t, string(serialized), "PRIVATE KEY")

	// Asserted on the value rather than the serialized form, which escapes the
	// angle brackets.
	fields, ok := audited.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "<redacted>", fields["node_credentials"])
}
