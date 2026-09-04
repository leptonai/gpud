// Package config provides the gpud configuration data for the server.
package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
)

// Config provides gpud configuration data for the server
type Config struct {
	APIVersion string `json:"api_version"`

	// Address for the server to listen on.
	Address string `json:"address"`

	// DataDir is the root directory for GPUd state and package artifacts.
	DataDir string `json:"data_dir"`

	// State file that persists the latest status.
	// If empty, the states are not persisted to file.
	State string `json:"state"`

	// Amount of time to retain states/metrics for.
	// Once elapsed, old states/metrics are purged/compacted.
	MetricsRetentionPeriod metav1.Duration `json:"metrics_retention_period"`

	// Amount of time to retain component events for.
	// Once elapsed, old events are purged from the event store.
	EventsRetentionPeriod metav1.Duration `json:"events_retention_period"`

	// Interval at which to compact the state database.
	CompactPeriod metav1.Duration `json:"compact_period"`

	// Set true to enable profiler.
	Pprof bool `json:"pprof"`

	// Set false to disable auto update
	EnableAutoUpdate bool `json:"enable_auto_update"`

	// Exit code to exit with when auto updating.
	// Only valid when the auto update is enabled.
	// Set -1 to disable the auto update by exit code.
	AutoUpdateExitCode int `json:"auto_update_exit_code"`

	// RebootCommands is a bash script to run when the control plane sends a reboot session request.
	// Empty preserves the built-in "sudo reboot" path.
	RebootCommands string `json:"reboot_commands,omitempty"`

	// FindmntCommands overrides how the disk component invokes "findmnt".
	// Empty preserves the legacy behavior of locating "findmnt" on PATH and
	// running it in the current namespace. When set (e.g.
	// "nsenter --target 1 --mount -- findmnt"), it runs in the host mount
	// namespace so the disk component reports the host's mounts.
	FindmntCommands string `json:"findmnt_commands,omitempty"`

	// LsblkCommands overrides how the disk component invokes "lsblk".
	// Empty preserves the legacy behavior. When set (e.g.
	// "nsenter --target 1 --mount -- lsblk"), it runs in the host mount namespace.
	LsblkCommands string `json:"lsblk_commands,omitempty"`

	// BlockdevUsageCommands overrides how the disk component collects partition
	// usage. Empty preserves the legacy behavior of enumerating mounts via
	// gopsutil and measuring usage via the statfs syscall. When set (e.g.
	// "nsenter --target 1 --mount -- df"), partitions and usage are read from
	// that command's output in the host mount namespace.
	BlockdevUsageCommands string `json:"blockdev_usage_commands,omitempty"`

	// NFSHostRoot prefixes NFS volume paths for file operations. Empty preserves
	// the existing direct-path behavior used by systemd installations. A
	// containerized deployment can use /proc/1/root to access the node filesystem
	// without mounting every node-group-specific NFS path into the container.
	NFSHostRoot string `json:"nfs_host_root,omitempty"`

	// ContainerdServiceActiveCommands overrides how the containerd component checks
	// whether the containerd service is active. Empty preserves the legacy
	// behavior of calling systemd directly (systemd.IsActive), which only sees the
	// container's own service manager. When set (e.g.
	// "nsenter --target 1 --mount -- systemctl is-active containerd"), the command
	// runs against the host's service manager; exit code 0 means active.
	ContainerdServiceActiveCommands string `json:"containerd_service_active_commands,omitempty"`

	// VersionFile is the file that contains the target version.
	// If empty, the version file is not used.
	VersionFile string `json:"version_file"`

	// A list of nvidia tool command paths to overwrite the default paths.
	NvidiaToolOverwrites pkgconfigcommon.ToolOverwrites `json:"nvidia_tool_overwrites"`

	// PluginSpecsFile is the file that contains the plugin specs.
	PluginSpecsFile string `json:"plugin_specs_file"`

	// NVSentinel configures the optional NVSentinel integration.
	// When enabled, GPUd serves the NVSentinel PlatformConnector gRPC API on
	// a local unix socket. The node's NVSentinel platform-connector forwards
	// health events to that socket. Components prefer the NVSentinel data
	// point when one exists and fall back to their own detection otherwise.
	NVSentinel *NVSentinelConfig `json:"nvsentinel,omitempty"`

	// Components specifies the components to enable.
	// Leave empty, "*", or "all" to enable all components.
	// Or prefix component names with "-" to disable them.
	Components         []string       `json:"components"`
	selectedComponents map[string]any `json:"-"`
	disabledComponents map[string]any `json:"-"`
	// allComponentsSelected is true when Components contains "*" or "all".
	allComponentsSelected bool `json:"-"`
	// hasComponentAllowlist is true when Components contains at least one
	// positive (non-"-") entry other than "*" / "all".
	hasComponentAllowlist bool `json:"-"`

	// FailureInjector is the failure injector.
	FailureInjector *components.FailureInjector `json:"failure_injector,omitempty"`

	// SkipSessionUpdateConfig skips processing of updateConfig session commands. Intended for testing.
	SkipSessionUpdateConfig bool `json:"skip_session_update_config"`

	// SessionProtocol selects the control-plane session transport: v1, v2, or auto.
	SessionProtocol string `json:"session_protocol"`

	// DBInMemory enables in-memory SQLite database mode.
	// When true, the database is opened as a shared in-memory database (file::memory:?cache=shared)
	// instead of using the State file path. Data will not persist across restarts.
	// ref. https://github.com/mattn/go-sqlite3?tab=readme-ov-file#faq
	DBInMemory bool `json:"db_in_memory"`

	// SessionToken is the session token for control plane authentication.
	// Used when DBInMemory is true and session credentials are passed via CLI flags.
	// This allows gpud up to pass the session token from login to gpud run.
	SessionToken string `json:"-"`

	// SessionMachineID is the machine ID assigned by the control plane.
	// Used when DBInMemory is true and session credentials are passed via CLI flags.
	// This allows gpud up to pass the assigned machine ID from login to gpud run.
	SessionMachineID string `json:"-"`

	// SessionMachineProof is the per-machine proof returned by login.
	SessionMachineProof string `json:"-"`

	// SessionEndpoint is the control plane endpoint.
	// Used when DBInMemory is true and session credentials are passed via CLI flags.
	// This allows gpud up to pass the endpoint from login to gpud run.
	// The server reads the endpoint from metadata DB, so it must be seeded for in-memory mode.
	SessionEndpoint string `json:"-"`
}

// NVSentinelConfig holds the top-level NVSentinel endpoint settings.
type NVSentinelConfig struct {
	// Enabled turns the NVSentinel receiver on. Default false.
	Enabled bool `json:"enabled"`

	// SocketPath is the unix socket GPUd serves for NVSentinel health event
	// forwarding. The NVSentinel platform-connector container must reach this
	// path. The default works with the stock NVSentinel DaemonSet, which
	// mounts the host /var/run/nvsentinel directory at container /var/run.
	// Empty means nvsentinel.DefaultSocketPath.
	SocketPath string `json:"socket_path,omitempty"`

	// EventDedupWindow is how long a received NVSentinel event suppresses
	// GPUd's own duplicate detection of the same data point. It needs only
	// to cover the delivery skew between the two detectors that watch the
	// same incident. Zero means nvsentinel.DefaultEventDedupWindow.
	EventDedupWindow metav1.Duration `json:"event_dedup_window,omitempty"`
}

func (config *Config) Validate() error {
	if config.Address == "" {
		return errors.New("address is required")
	}
	if config.MetricsRetentionPeriod.Duration < time.Minute {
		return fmt.Errorf("metrics_retention_period must be at least 1 minute, got %d", config.MetricsRetentionPeriod.Duration)
	}
	if config.EventsRetentionPeriod.Duration > 0 && config.EventsRetentionPeriod.Duration < time.Minute {
		return fmt.Errorf("events_retention_period must be at least 1 minute, got %d", config.EventsRetentionPeriod.Duration)
	}
	if config.NFSHostRoot != "" && !filepath.IsAbs(config.NFSHostRoot) {
		return fmt.Errorf("nfs_host_root must be an absolute path, got %q", config.NFSHostRoot)
	}
	if config.NVSentinel != nil {
		if config.NVSentinel.SocketPath != "" && !filepath.IsAbs(config.NVSentinel.SocketPath) {
			return fmt.Errorf("nvsentinel.socket_path must be an absolute path, got %q", config.NVSentinel.SocketPath)
		}
		if d := config.NVSentinel.EventDedupWindow.Duration; d < 0 {
			return fmt.Errorf("nvsentinel.event_dedup_window must not be negative, got %s", d)
		}
	}
	switch config.SessionProtocol {
	case "", "v1", "v2", "auto":
	default:
		return fmt.Errorf("session_protocol must be one of v1, v2, or auto, got %q", config.SessionProtocol)
	}

	return nil
}

// parseComponentSelectors lazily builds the allowlist and denylist lookup
// maps from Config.Components. Entries prefixed with "-" form the denylist,
// stored under their bare component names because callers (e.g. pkg/server)
// look components up by bare name. All other entries form the allowlist,
// where "*" and "all" select every component. A list containing only
// "-"-prefixed entries enables every component except the denylisted ones.
//
// Why stored bare (LEP-6439): the only production caller,
// pkg/server/server.go, invokes ShouldDisable(name) with the bare registered
// component name (e.g. "library"). The previous implementation stored the key
// as "-library" -- prefix included -- so the lookup could never match and the
// entire disable path was dead code. The old unit tests masked this by
// querying ShouldDisable("-comp1") with the prefix still attached, i.e. not
// mirroring the production call. Bug found in production on the AWS BYOK
// cluster aws-iad-nkxdev-1, where there was no working way to silence
// individual noisy components.
func (config *Config) parseComponentSelectors() {
	if config.selectedComponents != nil {
		return
	}
	config.selectedComponents = make(map[string]any)
	config.disabledComponents = make(map[string]any)
	for _, c := range config.Components {
		if name, ok := strings.CutPrefix(c, "-"); ok {
			config.disabledComponents[name] = struct{}{}
			continue
		}
		if c == "*" || c == "all" {
			config.allComponentsSelected = true
			continue
		}
		config.hasComponentAllowlist = true
		config.selectedComponents[c] = struct{}{}
	}
}

// ShouldEnable returns true if the component should be enabled.
// If the component list is not specified, it returns true, meaning every
// component is enabled by default. A list containing only "-"-prefixed
// entries enables every component except the denylisted ones. Otherwise the
// positive entries form an allowlist ("*" and "all" select every component),
// and "-"-prefixed entries subtract from the result in both modes.
func (config *Config) ShouldEnable(componentName string) bool {
	// not specified, thus enable all components
	if len(config.Components) == 0 {
		return true
	}

	config.parseComponentSelectors()

	// the denylist wins in both allowlist and denylist-only modes:
	// "--components=all,-foo" and "--components=-foo" both disable "foo".
	// (LEP-6439: the old "all"/"*" early return bailed out before "-" entries
	// were ever read, so "--components=all,-library" silently kept library
	// enabled -- exactly the combination the aws-iad-nkxdev-1 BYOK hot-fix
	// needed.)
	if _, disabled := config.disabledComponents[componentName]; disabled {
		return false
	}

	// "all"/"*", or a denylist-only list ("--components=-foo,-bar"):
	// everything not denylisted stays enabled. (LEP-6439: previously a
	// denylist-only list produced an EMPTY allowlist, so ShouldEnable returned
	// false for every component and pkg/server registered zero components --
	// the flag's own help text promises "-name" disables just that component.
	// Note "none" is a positive, non-matching entry, so the documented
	// "disable all components" escape hatch still works.)
	if config.allComponentsSelected || !config.hasComponentAllowlist {
		return true
	}

	// allowlist mode: only the listed components are enabled
	_, shouldEnable := config.selectedComponents[componentName]
	return shouldEnable
}

// ShouldDisable returns true if the component should be disabled.
// If the component list is not specified, it returns false, meaning no
// component is disabled by default.
func (config *Config) ShouldDisable(componentName string) bool {
	// not specified, thus enable all components (meaning should NOT disable any component)
	if len(config.Components) == 0 {
		return false
	}

	config.parseComponentSelectors()

	_, shouldDisable := config.disabledComponents[componentName]
	return shouldDisable
}
