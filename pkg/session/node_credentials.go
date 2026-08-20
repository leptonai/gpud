package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// nodeCredentialsAllowedPrefixes bounds where the control plane may place
// credential material.
//
// The destination travels in the request so that adding a credential kind, or
// moving one, does not need a GPUd release. That generality is why the prefixes
// exist: without them the method is an arbitrary root-owned file write, and a
// control plane that is wrong or compromised could replace a unit file or an
// authorized_keys. These two trees hold everything a node's identity is made
// of and nothing that grants code execution on its own.
var nodeCredentialsAllowedPrefixes = []string{
	"/var/lib/gpud/",
	"/etc/kubernetes/",
}

// allowedNodeCredentialPrefixes is a func so a test can point the allow-list at
// a temporary directory and exercise the real validation rather than a copy of
// it. Production always sees the list above.
var allowedNodeCredentialPrefixes = func() []string { return nodeCredentialsAllowedPrefixes }

// defaultNodeCredentialFileMode is what a credential gets when the request does
// not say. Private keys are the common case, so the default is owner-only.
const defaultNodeCredentialFileMode = 0o600

// NodeCredentialFile is one file to place on the node.
type NodeCredentialFile struct {
	// Path is absolute and must fall under an allowed prefix.
	Path string `json:"path"`
	// Contents is the file body. Empty contents are rejected: a credential
	// truncated to nothing looks like a successful write to the caller and
	// fails much later, wherever the file is actually read.
	Contents []byte `json:"contents"`
	// Mode defaults to 0600 when zero.
	Mode uint32 `json:"mode,omitempty"`
}

// NodeCredentialsRequest carries credential material for one node, grouped by
// the subsystem that owns it. Grouping rather than flattening means a new kind
// of credential is a new field, not a change to an existing one.
type NodeCredentialsRequest struct {
	// Kubelet is the material a kubelet needs to register: its client
	// certificate and key, its config, and whatever else the control plane
	// decides belongs with them.
	Kubelet []NodeCredentialFile `json:"kubelet,omitempty"`
}

// Files returns every file in the request, in the order they should be written.
func (r *NodeCredentialsRequest) Files() []NodeCredentialFile {
	if r == nil {
		return nil
	}
	return r.Kubelet
}

// processNodeCredentials writes credential material the control plane sends.
//
// It writes files and nothing else: it starts no process, installs no package
// and restarts no service. Whatever consumes the material decides when to act
// on it, which keeps this method safe to retry and safe to call on a node whose
// state the control plane cannot see.
//
// Every file is validated before any file is written, so a request with one bad
// path leaves the node exactly as it was rather than half-updated.
func (s *Session) processNodeCredentials(_ context.Context, request Request, response *Response) {
	if request.NodeCredentials == nil {
		response.Error = "node credentials are required"
		return
	}

	files := request.NodeCredentials.Files()
	if len(files) == 0 {
		response.Error = "node credentials contain no files"
		return
	}

	for _, file := range files {
		if err := validateNodeCredentialFile(file); err != nil {
			response.Error = err.Error()
			return
		}
	}

	for _, file := range files {
		if err := writeNodeCredentialFile(file); err != nil {
			response.Error = err.Error()
			return
		}
	}
}

func validateNodeCredentialFile(file NodeCredentialFile) error {
	if file.Path == "" {
		return errors.New("node credential path is required")
	}
	if !filepath.IsAbs(file.Path) {
		return fmt.Errorf("node credential path %q must be absolute", file.Path)
	}
	// Compared after cleaning so that a path spelled with ".." cannot leave an
	// allowed tree while still matching its prefix as written.
	cleaned := filepath.Clean(file.Path)
	allowed := false
	prefixes := allowedNodeCredentialPrefixes()
	for _, prefix := range prefixes {
		if strings.HasPrefix(cleaned, prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("node credential path %q is outside %s",
			file.Path, strings.Join(prefixes, ", "))
	}
	if len(file.Contents) == 0 {
		return fmt.Errorf("node credential %q has no contents", file.Path)
	}
	return nil
}

// writeNodeCredentialFile publishes one file by renaming it into place, so a
// reader either sees the previous contents or the new ones and never a partial
// write. The staging file is created in the destination directory to keep the
// rename on one filesystem, and with its final mode so the contents are never
// briefly readable under a permissive umask.
func writeNodeCredentialFile(file NodeCredentialFile) error {
	path := filepath.Clean(file.Path)
	mode := os.FileMode(defaultNodeCredentialFileMode)
	if file.Mode != 0 {
		mode = os.FileMode(file.Mode)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}

	staged, err := os.CreateTemp(dir, ".node-credential-*")
	if err != nil {
		return fmt.Errorf("failed to stage %s: %w", path, err)
	}
	stagedPath := staged.Name()
	defer func() {
		_ = os.Remove(stagedPath)
	}()

	if err := staged.Chmod(mode); err != nil {
		_ = staged.Close()
		return fmt.Errorf("failed to set mode on %s: %w", path, err)
	}
	if _, err := staged.Write(file.Contents); err != nil {
		_ = staged.Close()
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	// Flushed before the rename: a rename that outlives the data would publish
	// an empty file after a crash.
	if err := staged.Sync(); err != nil {
		_ = staged.Close()
		return fmt.Errorf("failed to flush %s: %w", path, err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("failed to close staged %s: %w", path, err)
	}
	if err := os.Rename(stagedPath, path); err != nil {
		return fmt.Errorf("failed to publish %s: %w", path, err)
	}
	return nil
}
