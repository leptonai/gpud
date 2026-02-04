package release

import (
	"crypto/ed25519"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/release/distsign"
)

// newGenKeyContext creates a CLI context for testing gen-key command.
func newGenKeyContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-release-gen-key-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.Bool("root", false, "")
	_ = flags.Bool("signing", false, "")
	_ = flags.String("priv-path", "", "")
	_ = flags.String("pub-path", "", "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

// newSignKeyContext creates a CLI context for testing sign-key command.
func newSignKeyContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-release-sign-key-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.String("root-priv-path", "", "")
	_ = flags.String("sign-pub-path", "", "")
	_ = flags.String("sig-path", "", "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

// newSignPackageContext creates a CLI context for testing sign-package command.
func newSignPackageContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-release-sign-package-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.String("sign-priv-path", "", "")
	_ = flags.String("package-path", "", "")
	_ = flags.String("sig-path", "", "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

// newVerifyKeySignatureContext creates a CLI context for testing verify-key-signature command.
func newVerifyKeySignatureContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-release-verify-key-sig-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.String("root-pub-path", "", "")
	_ = flags.String("sign-pub-path", "", "")
	_ = flags.String("sig-path", "", "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

// newVerifyPackageSignatureContext creates a CLI context for testing verify-package-signature command.
func newVerifyPackageSignatureContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-release-verify-pkg-sig-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.String("sign-pub-path", "", "")
	_ = flags.String("package-path", "", "")
	_ = flags.String("sig-path", "", "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

// =============================================================================
// CommandGenKey Tests
// =============================================================================

// TestCommandGenKey_InvalidLogLevel tests when an invalid log level is provided.
func TestCommandGenKey_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("gen-key command invalid log level", t, func() {
		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-release-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "invalid-level", "")
		_ = flags.Bool("root", true, "")
		_ = flags.Bool("signing", false, "")
		_ = flags.String("priv-path", "", "")
		_ = flags.String("pub-path", "", "")

		require.NoError(t, flags.Parse([]string{"--log-level", "invalid-level"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := CommandGenKey(cliContext)
		require.Error(t, err)
	})
}

// TestCommandGenKey_BothRootAndSigning tests when both root and signing flags are set.
func TestCommandGenKey_BothRootAndSigning(t *testing.T) {
	mockey.PatchConvey("gen-key command both root and signing", t, func() {
		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-release-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "info", "")
		_ = flags.Bool("root", true, "")
		_ = flags.Bool("signing", true, "")
		_ = flags.String("priv-path", "", "")
		_ = flags.String("pub-path", "", "")

		require.NoError(t, flags.Parse([]string{"--root", "--signing"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := CommandGenKey(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only one of --root or --signing can be set")
	})
}

// TestCommandGenKey_NeitherRootNorSigning tests when neither root nor signing flags are set.
func TestCommandGenKey_NeitherRootNorSigning(t *testing.T) {
	mockey.PatchConvey("gen-key command neither root nor signing", t, func() {
		cliContext := newGenKeyContext(t, []string{})

		err := CommandGenKey(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "set either --root or --signing")
	})
}

// TestCommandGenKey_RootKeyGenerationError tests when root key generation fails.
func TestCommandGenKey_RootKeyGenerationError(t *testing.T) {
	mockey.PatchConvey("gen-key command root key generation error", t, func() {
		mockey.Mock(distsign.GenerateRootKey).To(func() ([]byte, []byte, error) {
			return nil, nil, errors.New("failed to generate root key")
		}).Build()

		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-release-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "info", "")
		_ = flags.Bool("root", true, "")
		_ = flags.Bool("signing", false, "")
		_ = flags.String("priv-path", "", "")
		_ = flags.String("pub-path", "", "")

		require.NoError(t, flags.Parse([]string{"--root"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := CommandGenKey(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate root key")
	})
}

// TestCommandGenKey_SigningKeyGenerationError tests when signing key generation fails.
func TestCommandGenKey_SigningKeyGenerationError(t *testing.T) {
	mockey.PatchConvey("gen-key command signing key generation error", t, func() {
		mockey.Mock(distsign.GenerateSigningKey).To(func() ([]byte, []byte, error) {
			return nil, nil, errors.New("failed to generate signing key")
		}).Build()

		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-release-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "info", "")
		_ = flags.Bool("root", false, "")
		_ = flags.Bool("signing", true, "")
		_ = flags.String("priv-path", "", "")
		_ = flags.String("pub-path", "", "")

		require.NoError(t, flags.Parse([]string{"--signing"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := CommandGenKey(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate signing key")
	})
}

// TestCommandGenKey_RootKeySuccess tests successful root key generation.
func TestCommandGenKey_RootKeySuccess(t *testing.T) {
	mockey.PatchConvey("gen-key command root key success", t, func() {
		tmpDir := t.TempDir()
		privPath := filepath.Join(tmpDir, "root.priv")
		pubPath := filepath.Join(tmpDir, "root.pub")

		mockey.Mock(distsign.GenerateRootKey).To(func() ([]byte, []byte, error) {
			return []byte("mock-private-key"), []byte("mock-public-key"), nil
		}).Build()

		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-release-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "info", "")
		_ = flags.Bool("root", true, "")
		_ = flags.Bool("signing", false, "")
		_ = flags.String("priv-path", privPath, "")
		_ = flags.String("pub-path", pubPath, "")

		require.NoError(t, flags.Parse([]string{"--root", "--priv-path", privPath, "--pub-path", pubPath}))
		cliContext := cli.NewContext(app, flags, nil)

		err := CommandGenKey(cliContext)
		require.NoError(t, err)

		// Verify files were created
		privData, err := os.ReadFile(privPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("mock-private-key"), privData)

		pubData, err := os.ReadFile(pubPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("mock-public-key"), pubData)
	})
}

// TestCommandGenKey_SigningKeySuccess tests successful signing key generation.
func TestCommandGenKey_SigningKeySuccess(t *testing.T) {
	mockey.PatchConvey("gen-key command signing key success", t, func() {
		tmpDir := t.TempDir()
		privPath := filepath.Join(tmpDir, "signing.priv")
		pubPath := filepath.Join(tmpDir, "signing.pub")

		mockey.Mock(distsign.GenerateSigningKey).To(func() ([]byte, []byte, error) {
			return []byte("mock-signing-private-key"), []byte("mock-signing-public-key"), nil
		}).Build()

		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-release-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "info", "")
		_ = flags.Bool("root", false, "")
		_ = flags.Bool("signing", true, "")
		_ = flags.String("priv-path", privPath, "")
		_ = flags.String("pub-path", pubPath, "")

		require.NoError(t, flags.Parse([]string{"--signing", "--priv-path", privPath, "--pub-path", pubPath}))
		cliContext := cli.NewContext(app, flags, nil)

		err := CommandGenKey(cliContext)
		require.NoError(t, err)

		// Verify files were created
		privData, err := os.ReadFile(privPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("mock-signing-private-key"), privData)

		pubData, err := os.ReadFile(pubPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("mock-signing-public-key"), pubData)
	})
}

// =============================================================================
// CommandSignKey Tests
// =============================================================================

// TestCommandSignKey_InvalidLogLevel tests when an invalid log level is provided.
func TestCommandSignKey_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("sign-key command invalid log level", t, func() {
		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-release-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "invalid-level", "")
		_ = flags.String("root-priv-path", "", "")
		_ = flags.String("sign-pub-path", "", "")
		_ = flags.String("sig-path", "", "")

		require.NoError(t, flags.Parse([]string{"--log-level", "invalid-level"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := CommandSignKey(cliContext)
		require.Error(t, err)
	})
}

// TestCommandSignKey_RootPrivPathNotFound tests when root private key file is not found.
func TestCommandSignKey_RootPrivPathNotFound(t *testing.T) {
	mockey.PatchConvey("sign-key command root priv path not found", t, func() {
		cliContext := newSignKeyContext(t, []string{"--root-priv-path", "/nonexistent/root.priv"})

		err := CommandSignKey(cliContext)
		require.Error(t, err)
	})
}

// TestCommandSignKey_ParseRootKeyError tests when parsing root key fails.
func TestCommandSignKey_ParseRootKeyError(t *testing.T) {
	mockey.PatchConvey("sign-key command parse root key error", t, func() {
		tmpDir := t.TempDir()
		rootPrivPath := filepath.Join(tmpDir, "root.priv")
		require.NoError(t, os.WriteFile(rootPrivPath, []byte("invalid-key-data"), 0600))

		mockey.Mock(distsign.ParseRootKey).To(func(raw []byte) (*distsign.RootKey, error) {
			return nil, errors.New("invalid root key format")
		}).Build()

		cliContext := newSignKeyContext(t, []string{"--root-priv-path", rootPrivPath})

		err := CommandSignKey(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid root key format")
	})
}

// TestCommandSignKey_SignPubPathNotFound tests when signing public key file is not found.
func TestCommandSignKey_SignPubPathNotFound(t *testing.T) {
	mockey.PatchConvey("sign-key command sign pub path not found", t, func() {
		tmpDir := t.TempDir()
		rootPrivPath := filepath.Join(tmpDir, "root.priv")
		require.NoError(t, os.WriteFile(rootPrivPath, []byte("root-key-data"), 0600))

		mockey.Mock(distsign.ParseRootKey).To(func(raw []byte) (*distsign.RootKey, error) {
			return &distsign.RootKey{}, nil
		}).Build()

		cliContext := newSignKeyContext(t, []string{
			"--root-priv-path", rootPrivPath,
			"--sign-pub-path", "/nonexistent/sign.pub",
		})

		err := CommandSignKey(cliContext)
		require.Error(t, err)
	})
}

// =============================================================================
// CommandSignPackage Tests
// =============================================================================

// TestCommandSignPackage_InvalidLogLevel tests when an invalid log level is provided.
func TestCommandSignPackage_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("sign-package command invalid log level", t, func() {
		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-release-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "invalid-level", "")
		_ = flags.String("sign-priv-path", "", "")
		_ = flags.String("package-path", "", "")
		_ = flags.String("sig-path", "", "")

		require.NoError(t, flags.Parse([]string{"--log-level", "invalid-level"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := CommandSignPackage(cliContext)
		require.Error(t, err)
	})
}

// TestCommandSignPackage_SignPrivPathNotFound tests when signing private key file is not found.
func TestCommandSignPackage_SignPrivPathNotFound(t *testing.T) {
	mockey.PatchConvey("sign-package command sign priv path not found", t, func() {
		cliContext := newSignPackageContext(t, []string{"--sign-priv-path", "/nonexistent/sign.priv"})

		err := CommandSignPackage(cliContext)
		require.Error(t, err)
	})
}

// TestCommandSignPackage_ParseSigningKeyError tests when parsing signing key fails.
func TestCommandSignPackage_ParseSigningKeyError(t *testing.T) {
	mockey.PatchConvey("sign-package command parse signing key error", t, func() {
		tmpDir := t.TempDir()
		signPrivPath := filepath.Join(tmpDir, "sign.priv")
		require.NoError(t, os.WriteFile(signPrivPath, []byte("invalid-key-data"), 0600))

		mockey.Mock(distsign.ParseSigningKey).To(func(raw []byte) (*distsign.SigningKey, error) {
			return nil, errors.New("invalid signing key format")
		}).Build()

		cliContext := newSignPackageContext(t, []string{"--sign-priv-path", signPrivPath})

		err := CommandSignPackage(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid signing key format")
	})
}

// TestCommandSignPackage_PackagePathNotFound tests when package file is not found.
func TestCommandSignPackage_PackagePathNotFound(t *testing.T) {
	mockey.PatchConvey("sign-package command package path not found", t, func() {
		tmpDir := t.TempDir()
		signPrivPath := filepath.Join(tmpDir, "sign.priv")
		require.NoError(t, os.WriteFile(signPrivPath, []byte("sign-key-data"), 0600))

		mockey.Mock(distsign.ParseSigningKey).To(func(raw []byte) (*distsign.SigningKey, error) {
			return &distsign.SigningKey{}, nil
		}).Build()

		cliContext := newSignPackageContext(t, []string{
			"--sign-priv-path", signPrivPath,
			"--package-path", "/nonexistent/package.tar.gz",
		})

		err := CommandSignPackage(cliContext)
		require.Error(t, err)
	})
}

// =============================================================================
// CommandVerifyKeySignature Tests
// =============================================================================

// TestCommandVerifyKeySignature_InvalidLogLevel tests when an invalid log level is provided.
func TestCommandVerifyKeySignature_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("verify-key-signature command invalid log level", t, func() {
		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-release-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "invalid-level", "")
		_ = flags.String("root-pub-path", "", "")
		_ = flags.String("sign-pub-path", "", "")
		_ = flags.String("sig-path", "", "")

		require.NoError(t, flags.Parse([]string{"--log-level", "invalid-level"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := CommandVerifyKeySignature(cliContext)
		require.Error(t, err)
	})
}

// TestCommandVerifyKeySignature_RootPubPathNotFound tests when root public key file is not found.
func TestCommandVerifyKeySignature_RootPubPathNotFound(t *testing.T) {
	mockey.PatchConvey("verify-key-signature command root pub path not found", t, func() {
		cliContext := newVerifyKeySignatureContext(t, []string{"--root-pub-path", "/nonexistent/root.pub"})

		err := CommandVerifyKeySignature(cliContext)
		require.Error(t, err)
	})
}

// TestCommandVerifyKeySignature_ParseRootKeyBundleError tests when parsing root key bundle fails.
func TestCommandVerifyKeySignature_ParseRootKeyBundleError(t *testing.T) {
	mockey.PatchConvey("verify-key-signature command parse root key bundle error", t, func() {
		tmpDir := t.TempDir()
		rootPubPath := filepath.Join(tmpDir, "root.pub")
		require.NoError(t, os.WriteFile(rootPubPath, []byte("invalid-key-data"), 0600))

		mockey.Mock(distsign.ParseRootKeyBundle).To(func(raw []byte) ([]ed25519.PublicKey, error) {
			return nil, errors.New("invalid root key bundle format")
		}).Build()

		cliContext := newVerifyKeySignatureContext(t, []string{"--root-pub-path", rootPubPath})

		err := CommandVerifyKeySignature(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid root key bundle format")
	})
}

// TestCommandVerifyKeySignature_SignPubPathNotFound tests when signing public key file is not found.
func TestCommandVerifyKeySignature_SignPubPathNotFound(t *testing.T) {
	mockey.PatchConvey("verify-key-signature command sign pub path not found", t, func() {
		tmpDir := t.TempDir()
		rootPubPath := filepath.Join(tmpDir, "root.pub")
		require.NoError(t, os.WriteFile(rootPubPath, []byte("root-pub-data"), 0600))

		mockey.Mock(distsign.ParseRootKeyBundle).To(func(raw []byte) ([]ed25519.PublicKey, error) {
			return []ed25519.PublicKey{}, nil
		}).Build()

		cliContext := newVerifyKeySignatureContext(t, []string{
			"--root-pub-path", rootPubPath,
			"--sign-pub-path", "/nonexistent/sign.pub",
		})

		err := CommandVerifyKeySignature(cliContext)
		require.Error(t, err)
	})
}

// TestCommandVerifyKeySignature_SigPathNotFound tests when signature file is not found.
func TestCommandVerifyKeySignature_SigPathNotFound(t *testing.T) {
	mockey.PatchConvey("verify-key-signature command sig path not found", t, func() {
		tmpDir := t.TempDir()
		rootPubPath := filepath.Join(tmpDir, "root.pub")
		signPubPath := filepath.Join(tmpDir, "sign.pub")
		require.NoError(t, os.WriteFile(rootPubPath, []byte("root-pub-data"), 0600))
		require.NoError(t, os.WriteFile(signPubPath, []byte("sign-pub-data"), 0600))

		mockey.Mock(distsign.ParseRootKeyBundle).To(func(raw []byte) ([]ed25519.PublicKey, error) {
			return []ed25519.PublicKey{}, nil
		}).Build()

		cliContext := newVerifyKeySignatureContext(t, []string{
			"--root-pub-path", rootPubPath,
			"--sign-pub-path", signPubPath,
			"--sig-path", "/nonexistent/sig.bin",
		})

		err := CommandVerifyKeySignature(cliContext)
		require.Error(t, err)
	})
}

// TestCommandVerifyKeySignature_InvalidSignature tests when signature verification fails.
func TestCommandVerifyKeySignature_InvalidSignature(t *testing.T) {
	mockey.PatchConvey("verify-key-signature command invalid signature", t, func() {
		tmpDir := t.TempDir()
		rootPubPath := filepath.Join(tmpDir, "root.pub")
		signPubPath := filepath.Join(tmpDir, "sign.pub")
		sigPath := filepath.Join(tmpDir, "sig.bin")
		require.NoError(t, os.WriteFile(rootPubPath, []byte("root-pub-data"), 0600))
		require.NoError(t, os.WriteFile(signPubPath, []byte("sign-pub-data"), 0600))
		require.NoError(t, os.WriteFile(sigPath, []byte("invalid-sig"), 0600))

		mockey.Mock(distsign.ParseRootKeyBundle).To(func(raw []byte) ([]ed25519.PublicKey, error) {
			return []ed25519.PublicKey{}, nil
		}).Build()

		mockey.Mock(distsign.VerifyAny).To(func(keys []ed25519.PublicKey, msg, sig []byte) bool {
			return false
		}).Build()

		cliContext := newVerifyKeySignatureContext(t, []string{
			"--root-pub-path", rootPubPath,
			"--sign-pub-path", signPubPath,
			"--sig-path", sigPath,
		})

		err := CommandVerifyKeySignature(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "signature not valid")
	})
}

// TestCommandVerifyKeySignature_ValidSignature tests when signature verification succeeds.
func TestCommandVerifyKeySignature_ValidSignature(t *testing.T) {
	mockey.PatchConvey("verify-key-signature command valid signature", t, func() {
		tmpDir := t.TempDir()
		rootPubPath := filepath.Join(tmpDir, "root.pub")
		signPubPath := filepath.Join(tmpDir, "sign.pub")
		sigPath := filepath.Join(tmpDir, "sig.bin")
		require.NoError(t, os.WriteFile(rootPubPath, []byte("root-pub-data"), 0600))
		require.NoError(t, os.WriteFile(signPubPath, []byte("sign-pub-data"), 0600))
		require.NoError(t, os.WriteFile(sigPath, []byte("valid-sig"), 0600))

		mockey.Mock(distsign.ParseRootKeyBundle).To(func(raw []byte) ([]ed25519.PublicKey, error) {
			return []ed25519.PublicKey{}, nil
		}).Build()

		mockey.Mock(distsign.VerifyAny).To(func(keys []ed25519.PublicKey, msg, sig []byte) bool {
			return true
		}).Build()

		cliContext := newVerifyKeySignatureContext(t, []string{
			"--root-pub-path", rootPubPath,
			"--sign-pub-path", signPubPath,
			"--sig-path", sigPath,
		})

		err := CommandVerifyKeySignature(cliContext)
		require.NoError(t, err)
	})
}

// =============================================================================
// CommandVerifyPackageSignature Tests
// =============================================================================

// TestCommandVerifyPackageSignature_InvalidLogLevel tests when an invalid log level is provided.
func TestCommandVerifyPackageSignature_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("verify-package-signature command invalid log level", t, func() {
		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-release-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "invalid-level", "")
		_ = flags.String("sign-pub-path", "", "")
		_ = flags.String("package-path", "", "")
		_ = flags.String("sig-path", "", "")

		require.NoError(t, flags.Parse([]string{"--log-level", "invalid-level"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := CommandVerifyPackageSignature(cliContext)
		require.Error(t, err)
	})
}

// TestCommandVerifyPackageSignature_SignPubPathNotFound tests when signing public key file is not found.
func TestCommandVerifyPackageSignature_SignPubPathNotFound(t *testing.T) {
	mockey.PatchConvey("verify-package-signature command sign pub path not found", t, func() {
		cliContext := newVerifyPackageSignatureContext(t, []string{"--sign-pub-path", "/nonexistent/sign.pub"})

		err := CommandVerifyPackageSignature(cliContext)
		require.Error(t, err)
	})
}

// TestCommandVerifyPackageSignature_ParseSigningKeyBundleError tests when parsing signing key bundle fails.
func TestCommandVerifyPackageSignature_ParseSigningKeyBundleError(t *testing.T) {
	mockey.PatchConvey("verify-package-signature command parse signing key bundle error", t, func() {
		tmpDir := t.TempDir()
		signPubPath := filepath.Join(tmpDir, "sign.pub")
		require.NoError(t, os.WriteFile(signPubPath, []byte("invalid-key-data"), 0600))

		mockey.Mock(distsign.ParseSigningKeyBundle).To(func(raw []byte) ([]ed25519.PublicKey, error) {
			return nil, errors.New("invalid signing key bundle format")
		}).Build()

		cliContext := newVerifyPackageSignatureContext(t, []string{"--sign-pub-path", signPubPath})

		err := CommandVerifyPackageSignature(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid signing key bundle format")
	})
}

// TestCommandVerifyPackageSignature_PackagePathNotFound tests when package file is not found.
func TestCommandVerifyPackageSignature_PackagePathNotFound(t *testing.T) {
	mockey.PatchConvey("verify-package-signature command package path not found", t, func() {
		tmpDir := t.TempDir()
		signPubPath := filepath.Join(tmpDir, "sign.pub")
		require.NoError(t, os.WriteFile(signPubPath, []byte("sign-pub-data"), 0600))

		mockey.Mock(distsign.ParseSigningKeyBundle).To(func(raw []byte) ([]ed25519.PublicKey, error) {
			return []ed25519.PublicKey{}, nil
		}).Build()

		cliContext := newVerifyPackageSignatureContext(t, []string{
			"--sign-pub-path", signPubPath,
			"--package-path", "/nonexistent/package.tar.gz",
		})

		err := CommandVerifyPackageSignature(cliContext)
		require.Error(t, err)
	})
}

// TestCommandVerifyPackageSignature_SigPathNotFound tests when signature file is not found.
func TestCommandVerifyPackageSignature_SigPathNotFound(t *testing.T) {
	mockey.PatchConvey("verify-package-signature command sig path not found", t, func() {
		tmpDir := t.TempDir()
		signPubPath := filepath.Join(tmpDir, "sign.pub")
		packagePath := filepath.Join(tmpDir, "package.tar.gz")
		require.NoError(t, os.WriteFile(signPubPath, []byte("sign-pub-data"), 0600))
		require.NoError(t, os.WriteFile(packagePath, []byte("package-content"), 0600))

		mockey.Mock(distsign.ParseSigningKeyBundle).To(func(raw []byte) ([]ed25519.PublicKey, error) {
			return []ed25519.PublicKey{}, nil
		}).Build()

		cliContext := newVerifyPackageSignatureContext(t, []string{
			"--sign-pub-path", signPubPath,
			"--package-path", packagePath,
			"--sig-path", "/nonexistent/sig.bin",
		})

		err := CommandVerifyPackageSignature(cliContext)
		require.Error(t, err)
	})
}

// TestCommandVerifyPackageSignature_InvalidSignature tests when signature verification fails.
func TestCommandVerifyPackageSignature_InvalidSignature(t *testing.T) {
	mockey.PatchConvey("verify-package-signature command invalid signature", t, func() {
		tmpDir := t.TempDir()
		signPubPath := filepath.Join(tmpDir, "sign.pub")
		packagePath := filepath.Join(tmpDir, "package.tar.gz")
		sigPath := filepath.Join(tmpDir, "sig.bin")
		require.NoError(t, os.WriteFile(signPubPath, []byte("sign-pub-data"), 0600))
		require.NoError(t, os.WriteFile(packagePath, []byte("package-content"), 0600))
		require.NoError(t, os.WriteFile(sigPath, []byte("invalid-sig"), 0600))

		mockey.Mock(distsign.ParseSigningKeyBundle).To(func(raw []byte) ([]ed25519.PublicKey, error) {
			return []ed25519.PublicKey{}, nil
		}).Build()

		mockey.Mock(distsign.VerifyAny).To(func(keys []ed25519.PublicKey, msg, sig []byte) bool {
			return false
		}).Build()

		cliContext := newVerifyPackageSignatureContext(t, []string{
			"--sign-pub-path", signPubPath,
			"--package-path", packagePath,
			"--sig-path", sigPath,
		})

		err := CommandVerifyPackageSignature(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "signature not valid")
	})
}

// TestCommandVerifyPackageSignature_ValidSignature tests when signature verification succeeds.
func TestCommandVerifyPackageSignature_ValidSignature(t *testing.T) {
	mockey.PatchConvey("verify-package-signature command valid signature", t, func() {
		tmpDir := t.TempDir()
		signPubPath := filepath.Join(tmpDir, "sign.pub")
		packagePath := filepath.Join(tmpDir, "package.tar.gz")
		sigPath := filepath.Join(tmpDir, "sig.bin")
		require.NoError(t, os.WriteFile(signPubPath, []byte("sign-pub-data"), 0600))
		require.NoError(t, os.WriteFile(packagePath, []byte("package-content"), 0600))
		require.NoError(t, os.WriteFile(sigPath, []byte("valid-sig"), 0600))

		mockey.Mock(distsign.ParseSigningKeyBundle).To(func(raw []byte) ([]ed25519.PublicKey, error) {
			return []ed25519.PublicKey{}, nil
		}).Build()

		mockey.Mock(distsign.VerifyAny).To(func(keys []ed25519.PublicKey, msg, sig []byte) bool {
			return true
		}).Build()

		cliContext := newVerifyPackageSignatureContext(t, []string{
			"--sign-pub-path", signPubPath,
			"--package-path", packagePath,
			"--sig-path", sigPath,
		})

		err := CommandVerifyPackageSignature(cliContext)
		require.NoError(t, err)
	})
}

// =============================================================================
// Valid Log Level Tests
// =============================================================================

// TestCommandGenKey_ValidLogLevels tests that valid log levels are accepted.
func TestCommandGenKey_ValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			mockey.PatchConvey("gen-key valid log level "+level, t, func() {
				app := cli.NewApp()
				flags := flag.NewFlagSet("gpud-release-test", flag.ContinueOnError)
				flags.SetOutput(io.Discard)

				_ = flags.String("log-level", level, "")
				_ = flags.Bool("root", false, "")
				_ = flags.Bool("signing", false, "")
				_ = flags.String("priv-path", "", "")
				_ = flags.String("pub-path", "", "")

				require.NoError(t, flags.Parse([]string{"--log-level", level}))
				cliContext := cli.NewContext(app, flags, nil)

				// Will fail on flag validation, but log level parsing should succeed
				err := CommandGenKey(cliContext)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "set either --root or --signing")
			})
		})
	}
}
