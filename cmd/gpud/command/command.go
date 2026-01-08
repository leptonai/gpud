package command

import (
	"fmt"

	"github.com/urfave/cli"

	cmdcompact "github.com/leptonai/gpud/cmd/gpud/compact"
	cmdcustomplugins "github.com/leptonai/gpud/cmd/gpud/custom-plugins"
	cmddown "github.com/leptonai/gpud/cmd/gpud/down"
	cmdinjectfault "github.com/leptonai/gpud/cmd/gpud/inject-fault"
	cmdlistplugins "github.com/leptonai/gpud/cmd/gpud/list-plugins"
	cmdmachineinfo "github.com/leptonai/gpud/cmd/gpud/machine-info"
	cmdmetadata "github.com/leptonai/gpud/cmd/gpud/metadata"
	cmdnotify "github.com/leptonai/gpud/cmd/gpud/notify"
	cmdrelease "github.com/leptonai/gpud/cmd/gpud/release"
	cmdrun "github.com/leptonai/gpud/cmd/gpud/run"
	cmdrunplugingroup "github.com/leptonai/gpud/cmd/gpud/run-plugin-group"
	cmdscan "github.com/leptonai/gpud/cmd/gpud/scan"
	cmdsethealthy "github.com/leptonai/gpud/cmd/gpud/set-healthy"
	cmdstatus "github.com/leptonai/gpud/cmd/gpud/status"
	cmdup "github.com/leptonai/gpud/cmd/gpud/up"
	cmdupdate "github.com/leptonai/gpud/cmd/gpud/update"
	componentsxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	pkgconfig "github.com/leptonai/gpud/pkg/config"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
	"github.com/leptonai/gpud/version"
)

const usage = `
# to quick scan for your machine health status
gpud scan

# to start gpud as a systemd unit
sudo gpud up
`

func App() *cli.App {
	app := cli.NewApp()

	app.Name = "gpud"
	app.Version = version.Version
	app.Usage = usage
	app.Description = "GPU health checkers"

	app.Commands = []cli.Command{
		{
			Name:  "up",
			Usage: "initialize and start gpud in a daemon mode (systemd)",
			UsageText: `# to start gpud as a systemd unit (recommended)
sudo gpud up

# to enable machine monitoring powered by https://www.nvidia.com/en-us/data-center/dgx-cloud-lepton platform
# sign up here: https://www.nvidia.com/en-us/data-center/dgx-cloud-lepton
sudo gpud up --token <LEPTON_AI_TOKEN>

# to start gpud without a systemd unit (e.g., mac)
gpud run

# or
nohup sudo gpud run &>> <your log file path> &
`,
			Action: cmdup.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "data-dir",
					Usage: "set the data directory for GPUd state and packages (default: /var/lib/gpud or ~/.gpud for non-root)",
				},
				&cli.BoolFlag{
					Name:  "db-in-memory",
					Usage: "use in-memory SQLite database (file::memory:?cache=shared) instead of file-based storage; overrides --data-dir for database",
				},
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},

				// optional, only required for logging into platform/control plane
				cli.StringFlag{
					Name:  "token",
					Usage: "(optional) https://www.nvidia.com/en-us/data-center/dgx-cloud-lepton workspace token for checking in",
				},
				cli.StringFlag{
					Name:  "private-ip",
					Usage: "(optional) can specify private ip for internal network",
				},
				cli.StringFlag{
					Name:  "public-ip",
					Usage: "(optional) can specify public ip for machine",
				},
				cli.StringFlag{
					Name:   "machine-id",
					Hidden: true,
					Usage:  "(optional) for override default machine id",
				},
				cli.StringFlag{
					Name:  "node-group",
					Usage: "(optional) node group to join",
				},
				cli.StringFlag{
					Name:  "endpoint",
					Usage: "(optional) endpoint for checking in",
					Value: "gpud-manager-prod01.dgxc-lepton.nvidia.com",
				},
				cli.StringFlag{
					Name:  "gpu-count",
					Usage: "(optional) specify count of gpu (leave empty to auto-detect)",
				},
			},
		},
		{
			Name:  "down",
			Usage: "stop gpud systemd unit",
			UsageText: `# to stop the existing gpud systemd unit
sudo gpud down

# to uninstall gpud
sudo rm /usr/local/bin
sudo rm /etc/systemd/system/gpud.service
`,
			Action: cmddown.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "data-dir",
					Usage: "set the data directory for GPUd state and packages (default: /var/lib/gpud or ~/.gpud for non-root)",
				},
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				&cli.BoolFlag{
					Name:  "reset-state",
					Usage: "reset the state file (otherwise, re-login may contain stale health data)",
				},
				&cli.BoolFlag{
					Name:  "cleanup-packages",
					Usage: "run 'init.sh delete' for packages with needDelete markers in <data-dir>/packages/ (only works if machine is in deleting stage)",
				},
			},
		},
		{
			Name:   "run",
			Usage:  "starts gpud without any login/checkin ('gpud up' is recommended for linux) -- if --token is provided, it will perform login",
			Action: cmdrun.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "data-dir",
					Usage: "set the data directory for GPUd state and packages (default: /var/lib/gpud or ~/.gpud for non-root)",
				},
				&cli.BoolFlag{
					Name:  "db-in-memory",
					Usage: "use in-memory SQLite database (file::memory:?cache=shared) instead of file-based storage; overrides --data-dir for database",
				},
				&cli.StringFlag{
					Name:   "endpoint",
					Usage:  "(optional) endpoint for control plane",
					Hidden: true,
				},
				&cli.StringFlag{
					Name:   "machine-id",
					Hidden: true,
					Usage:  "(optional) for override default machine id",
				},
				&cli.StringFlag{
					Name:   "token",
					Hidden: true,
					Usage:  "(optional) token for control plane",
				},
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				&cli.StringFlag{
					Name:  "log-file",
					Usage: "set the log file path (set empty to stdout/stderr)",
					Value: "",
				},
				&cli.StringFlag{
					Name:  "listen-address",
					Usage: "set the listen address",
					Value: fmt.Sprintf("0.0.0.0:%d", pkgconfig.DefaultGPUdPort),
				},
				&cli.BoolFlag{
					Name:  "pprof",
					Usage: "enable pprof (default: false)",
				},
				&cli.DurationFlag{
					Name:  "retention-period",
					Usage: "set the time period to retain metrics for (once elapsed, old records are compacted/purged)",
					Value: pkgconfig.DefaultRetentionPeriod.Duration,
				},
				&cli.BoolTFlag{
					Name:  "enable-auto-update",
					Usage: "enable auto update of gpud (default: true)",
				},
				&cli.IntFlag{
					Name:  "auto-update-exit-code",
					Usage: "specifies the exit code to exit with when auto updating (set -1 to disable exit code)",
				},
				cli.StringFlag{
					Name:  "version-file",
					Usage: "specifies the version file to use for auto update (leave empty to disable auto update)",
					Value: pkgupdate.DefaultVersionFile,
				},
				cli.StringFlag{
					Name:  "plugin-specs-file",
					Usage: "sets the plugin specs file (leave empty for default) -- if the file does not exist, gpud does not install/run any plugin, and updated configuration requires an gpud restart)",
					Value: pkgcustomplugins.DefaultPluginSpecsFile,
				},
				cli.StringFlag{
					Name:  "components",
					Usage: "sets the components to enable (comma-separated, leave empty for default to enable all components, set 'none' or any other non-matching value to disable all components, prefix component name with '-' to disable it)",
					Value: "",
				},
				&cli.BoolFlag{
					Name:   "skip-session-update-config",
					Usage:  "skips processing session updateConfig requests (testing only)",
					Hidden: true,
				},

				&cli.IntFlag{
					Name:  "gpu-count",
					Usage: "specifies the expected GPU count",
					Value: 0,
				},
				&cli.StringFlag{
					Name:  "infiniband-expected-port-states",
					Usage: "set the infiniband expected port states in JSON (leave empty for default, useful for testing)",
				},
				&cli.StringFlag{
					Name:  "nvlink-expected-link-states",
					Usage: "set the nvlink expected link states in JSON (leave empty for default, useful for testing)",
				},
				&cli.StringFlag{
					Name:  "nfs-checker-configs",
					Usage: "set the NFS checker group configs in JSON (leave empty for default, useful for testing)",
				},
				&cli.IntFlag{
					Name:  "xid-reboot-threshold",
					Usage: fmt.Sprintf("set the allowed reboot attempts for XID errors before escalation (defaults to %d)", componentsxid.DefaultRebootThreshold),
					Value: componentsxid.DefaultRebootThreshold,
				},

				cli.StringFlag{
					Name:  "infiniband-exclude-devices",
					Usage: "comma-separated list of InfiniBand device names to exclude from monitoring (e.g., 'mlx5_0,mlx5_1'). Use this to skip devices with restricted Physical Functions (PFs) that cause kernel errors (mlx5_cmd_out_err ACCESS_REG). Common on NVIDIA DGX, Umbriel, and GB200 systems. See https://github.com/leptonai/gpud/issues/1164",
					Value: "",
				},
				cli.StringFlag{
					Name:   "infiniband-class-root-dir",
					Usage:  "(testing purposes) sets the infiniband class root directory (leave empty for default)",
					Value:  "",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-product-name",
					Usage:  "(testing purposes) set the gpu product name to overwrite",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-row-remapping-pending",
					Usage:  "(testing purposes) set the comma-separated gpu uuids with row remapping pending",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-row-remapping-failed",
					Usage:  "(testing purposes) set the comma-separated gpu uuids with row remapping failed",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-hw-slowdown",
					Usage:  "(testing purposes) set the comma-separated gpu uuids with hw slowdown",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-hw-slowdown-thermal",
					Usage:  "(testing purposes) set the comma-separated gpu uuids with hw slowdown thermal",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-hw-slowdown-power-brake",
					Usage:  "(testing purposes) set the comma-separated gpu uuids with hw slowdown power brake",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-gpu-lost",
					Usage:  "(testing purposes) set the comma-separated gpu uuids to return 'GPU lost' NVML error (nvml.ERROR_GPU_IS_LOST)",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-gpu-requires-reset",
					Usage:  "(testing purposes) set the comma-separated gpu uuids to return 'GPU requires reset' NVML error (nvml.ERROR_GPU_REQUIRES_RESET)",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-fabric-state-health-summary-unhealthy",
					Usage:  "(testing purposes) set the comma-separated gpu uuids to return GPU fabric health summary unhealthy (nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY). NOTE: Only works on multi-GPU NVSwitch systems (H100-SXM, H200-SXM, GB200). Ignored on PCIe variants and single-GPU systems.",
					Hidden: true, // only for testing
				},
			},
		},
		{
			Name:      "update",
			Usage:     "update gpud",
			UsageText: "",
			Action:    cmdupdate.Command,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				cli.StringFlag{
					Name:  "url",
					Usage: "url for getting a package",
				},
				cli.StringFlag{
					Name:  "next-version",
					Usage: "set the next version",
				},
			},
			Subcommands: []cli.Command{
				{
					Name:   "check",
					Usage:  "check availability of new version gpud",
					Action: cmdupdate.CommandCheck,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "log-level,l",
							Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
						},
					},
				},
			},
		},
		{
			Name:  "release",
			Usage: "release gpud",
			Subcommands: []cli.Command{
				{
					Name:   "gen-key",
					Usage:  "generate root or signing key pair",
					Action: cmdrelease.CommandGenKey,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "log-level,l",
							Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
						},
						cli.BoolFlag{
							Name:  "root (default: false)",
							Usage: "generate root key",
						},
						cli.BoolFlag{
							Name:  "signing (default: false)",
							Usage: "generate signing key",
						},
						cli.StringFlag{
							Name:  "priv-path",
							Usage: "path of private key",
						},
						cli.StringFlag{
							Name:  "pub-path",
							Usage: "path of public key",
						},
					},
				},
				{
					Name:   "sign-key",
					Usage:  "Sign signing keys with a root key",
					Action: cmdrelease.CommandSignKey,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "log-level,l",
							Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
						},
						cli.StringFlag{
							Name:  "root-priv-path",
							Usage: "path of root private key",
						},
						cli.StringFlag{
							Name:  "sign-pub-path",
							Usage: "path of signing public key",
						},
						cli.StringFlag{
							Name:  "sig-path",
							Usage: "output path of signature path",
						},
					},
				},
				{
					Name:   "verify-key-signature",
					Usage:  "Verify a root signture of the signing keys' bundle",
					Action: cmdrelease.CommandVerifyKeySignature,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "log-level,l",
							Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
						},
						cli.StringFlag{
							Name:  "root-pub-path",
							Usage: "path of root public key",
						},
						cli.StringFlag{
							Name:  "sign-pub-path",
							Usage: "path of signing public key",
						},
						cli.StringFlag{
							Name:  "sig-path",
							Usage: "path of signature path",
						},
					},
				},
				{
					Name:   "sign-package",
					Usage:  "Sign a package with a signing key",
					Action: cmdrelease.CommandSignPackage,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "log-level,l",
							Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
						},
						cli.StringFlag{
							Name:  "package-path",
							Usage: "path of package",
						},
						cli.StringFlag{
							Name:  "sign-priv-path",
							Usage: "path of signing private key",
						},
						cli.StringFlag{
							Name:  "sig-path",
							Usage: "output path of signature path",
						},
					},
				},
				{
					Name:   "verify-package-signature",
					Usage:  "Verify a package signture using a signing key",
					Action: cmdrelease.CommandVerifyPackageSignature,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "log-level,l",
							Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
						},
						cli.StringFlag{
							Name:  "package-path",
							Usage: "path of package",
						},
						cli.StringFlag{
							Name:  "sign-pub-path",
							Usage: "path of signing public key",
						},
						cli.StringFlag{
							Name:  "sig-path",
							Usage: "path of signature path",
						},
					},
				},
			},
		},
		{
			Name:    "notify",
			Aliases: []string{"nt"},
			Usage:   "notify control plane of state change",
			Subcommands: []cli.Command{
				{
					Name:   "startup",
					Usage:  "notify machine startup",
					Action: cmdnotify.CommandStartup,
					Flags: []cli.Flag{
						&cli.StringFlag{
							Name:  "data-dir",
							Usage: "set the data directory for GPUd state and packages (default: /var/lib/gpud or ~/.gpud for non-root)",
						},
						&cli.StringFlag{
							Name:  "log-level,l",
							Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
						},
					},
				},
				{
					Name:   "shutdown",
					Usage:  "notify machine shutdown",
					Action: cmdnotify.CommandShutdown,
					Flags: []cli.Flag{
						&cli.StringFlag{
							Name:  "data-dir",
							Usage: "set the data directory for GPUd state and packages (default: /var/lib/gpud or ~/.gpud for non-root)",
						},
						&cli.StringFlag{
							Name:  "log-level,l",
							Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
						},
					},
				},
			},
		},
		{
			Name:    "status",
			Aliases: []string{"st"},
			Usage:   "checks the status of gpud",
			Action:  cmdstatus.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "data-dir",
					Usage: "set the data directory for GPUd state and packages (default: /var/lib/gpud or ~/.gpud for non-root)",
				},
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				&cli.BoolFlag{
					Name:  "watch,w",
					Usage: "watch for package install status",
				},
			},
		},
		{
			Name:   "compact",
			Usage:  "compact the GPUd state database to reduce the size in disk (GPUd must be stopped)",
			Action: cmdcompact.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "data-dir",
					Usage: "set the data directory for GPUd state and packages (default: /var/lib/gpud or ~/.gpud for non-root)",
				},
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
			},
		},
		{
			Name:    "scan",
			Aliases: []string{"check", "s"},
			Usage:   "quick scans the host for any major issues",
			Action:  cmdscan.CreateCommand(),
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},

				&cli.IntFlag{
					Name:  "gpu-count",
					Usage: "specifies the expected GPU count",
					Value: 0,
				},
				&cli.StringFlag{
					Name:  "infiniband-expected-port-states",
					Usage: "set the infiniband expected port states in JSON (leave empty for default, useful for testing)",
				},
				&cli.StringFlag{
					Name:  "nvlink-expected-link-states",
					Usage: "set the nvlink expected link states in JSON (leave empty for default, useful for testing)",
				},
				&cli.StringFlag{
					Name:  "nfs-checker-configs",
					Usage: "set the NFS checker group configs in JSON (leave empty for default, useful for testing)",
				},
				&cli.IntFlag{
					Name:  "xid-reboot-threshold",
					Usage: fmt.Sprintf("set the allowed reboot attempts for XID errors before escalation (defaults to %d)", componentsxid.DefaultRebootThreshold),
					Value: componentsxid.DefaultRebootThreshold,
				},
				cli.StringFlag{
					Name:   "infiniband-class-root-dir",
					Usage:  "sets the infiniband class root directory (leave empty for default)",
					Value:  "",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:  "infiniband-exclude-devices",
					Usage: "comma-separated list of InfiniBand device names to exclude from monitoring (e.g., 'mlx5_0,mlx5_1'). Use this to skip devices with restricted Physical Functions (PFs) that cause kernel errors (mlx5_cmd_out_err ACCESS_REG). Common on NVIDIA DGX, Umbriel, and GB200 systems. See https://github.com/leptonai/gpud/issues/1164",
					Value: "",
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-row-remapping-pending",
					Usage:  "set the comma-separated gpu uuids with row remapping pending",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-row-remapping-failed",
					Usage:  "set the comma-separated gpu uuids with row remapping failed",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-hw-slowdown",
					Usage:  "set the comma-separated gpu uuids with hw slowdown",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-hw-slowdown-thermal",
					Usage:  "set the comma-separated gpu uuids with hw slowdown thermal",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-hw-slowdown-power-brake",
					Usage:  "set the comma-separated gpu uuids with hw slowdown power brake",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-gpu-lost",
					Usage:  "(testing purposes) set the comma-separated gpu uuids to return 'GPU lost' NVML error",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-gpu-requires-reset",
					Usage:  "(testing purposes) set the comma-separated gpu uuids to return 'GPU requires reset' NVML error",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-uuids-with-fabric-state-health-summary-unhealthy",
					Usage:  "(testing purposes) set the comma-separated gpu uuids to return GPU fabric health summary unhealthy. NOTE: Only works on multi-GPU NVSwitch systems (H100-SXM, H200-SXM, GB200). Use --gpu-product-name to override.",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:   "gpu-product-name",
					Usage:  "(testing purposes) override the detected GPU product name to simulate different GPU types (e.g., set 'H100-SXM' on H100-PCIe to enable fabric state testing)",
					Hidden: true, // only for testing
				},
			},
		},
		{
			Name:    "list-plugins",
			Aliases: []string{"lp"},
			Usage:   "list all registered custom plugins",
			Action:  cmdlistplugins.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				&cli.StringFlag{
					Name:  "server",
					Usage: "server address for control plane",
				},
			},
		},
		{
			Name:    "custom-plugins",
			Aliases: []string{"cs", "plugin", "plugins"},
			Usage:   "checks/runs custom plugins",
			Action:  cmdcustomplugins.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				&cli.BoolFlag{
					Name:  "run,r",
					Usage: "run the custom plugins (default: false)",
				},
				&cli.BoolTFlag{
					Name:  "fail-fast,f",
					Usage: "fail fast, exit immediately if any plugin returns unhealthy state (default: true)",
				},
				cli.StringFlag{
					Name:   "infiniband-class-root-dir",
					Usage:  "sets the infiniband class root directory (leave empty for default)",
					Value:  "",
					Hidden: true, // only for testing
				},
				cli.StringFlag{
					Name:  "infiniband-exclude-devices",
					Usage: "comma-separated list of InfiniBand device names to exclude from monitoring (e.g., 'mlx5_0,mlx5_1'). Use this to skip devices with restricted Physical Functions (PFs) that cause kernel errors (mlx5_cmd_out_err ACCESS_REG). Common on NVIDIA DGX, Umbriel, and GB200 systems. See https://github.com/leptonai/gpud/issues/1164",
					Value: "",
				},
			},
		},
		{
			Name:      "run-plugin-group",
			Usage:     "Run all components in a plugin group by tag",
			UsageText: "gpud run-plugin-group <plugin_group_name>",
			Action:    cmdrunplugingroup.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				&cli.StringFlag{
					Name:  "server",
					Usage: "server address for control plane",
				},
			},
		},
		{
			Name:      "machine-info",
			Usage:     "get machine info (useful for debugging)",
			UsageText: "gpud machine-info",
			Action:    cmdmachineinfo.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "data-dir",
					Usage: "set the data directory for GPUd state and packages (default: /var/lib/gpud or ~/.gpud for non-root)",
				},
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
			},
		},
		{
			Name:   "inject-fault",
			Usage:  "injects a fault such as writing a kernel message to the kernel log",
			Action: cmdinjectfault.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				&cli.StringFlag{
					Name:  "kernel-log-level",
					Usage: "set the kernel log level [KERN_EMERG, KERN_ALERT, KERN_CRIT, KERN_ERR, KERN_WARNING, KERN_NOTICE, KERN_INFO, KERN_DEBUG]",
					Value: "KERN_INFO",
				},
				&cli.StringFlag{
					Name:  "kernel-message",
					Usage: "set the kernel message to inject",
				},
			},
		},
		{
			Name:      "set-healthy",
			Aliases:   []string{"set-health"},
			Usage:     "set the healthy state of components",
			UsageText: "gpud set-healthy <components> [options]\n\n   <components>: comma-separated list of component names to set healthy (if empty, sets all components).",
			Action:    cmdsethealthy.CreateCommand(),
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				&cli.StringFlag{
					Name:  "server",
					Usage: "server address for GPUd API (default: https://localhost:15132)",
				},
			},
		},
		{
			Name:   "metadata",
			Usage:  "inspects/updates metadata table",
			Action: cmdmetadata.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "data-dir",
					Usage: "set the data directory for GPUd state and packages (default: /var/lib/gpud or ~/.gpud for non-root)",
				},
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				&cli.BoolFlag{
					Name:  "reboot-history",
					Usage: "show reboot history recorded by gpud (default: false)",
				},
				&cli.StringFlag{
					Name:  "set-key",
					Usage: "metadata key to set/update",
				},
				&cli.StringFlag{
					Name:  "set-value",
					Usage: "value to set for the metadata key",
				},
			},
		},
	}

	return app
}
