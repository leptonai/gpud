package command

import (
	"fmt"

	"github.com/urfave/cli"

	cmdcompact "github.com/leptonai/gpud/cmd/gpud/compact"
	cmdcustomplugins "github.com/leptonai/gpud/cmd/gpud/custom-plugins"
	cmddown "github.com/leptonai/gpud/cmd/gpud/down"
	cmdinjectfault "github.com/leptonai/gpud/cmd/gpud/inject-fault"
	cmdjoin "github.com/leptonai/gpud/cmd/gpud/join"
	cmdlistplugins "github.com/leptonai/gpud/cmd/gpud/list-plugins"
	cmdlogin "github.com/leptonai/gpud/cmd/gpud/login"
	cmdmachineinfo "github.com/leptonai/gpud/cmd/gpud/machine-info"
	cmdnotify "github.com/leptonai/gpud/cmd/gpud/notify"
	cmdrelease "github.com/leptonai/gpud/cmd/gpud/release"
	cmdrun "github.com/leptonai/gpud/cmd/gpud/run"
	cmdrunplugingroup "github.com/leptonai/gpud/cmd/gpud/run-plugin-group"
	cmdscan "github.com/leptonai/gpud/cmd/gpud/scan"
	cmdstatus "github.com/leptonai/gpud/cmd/gpud/status"
	cmdup "github.com/leptonai/gpud/cmd/gpud/up"
	cmdupdate "github.com/leptonai/gpud/cmd/gpud/update"
	"github.com/leptonai/gpud/pkg/config"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
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
	app.Description = "monitor your GPU/CPU machines and run workloads"

	app.Commands = []cli.Command{
		{
			Name:  "login",
			Usage: "login gpud to lepton.ai (called automatically in gpud up with non-empty --token)",
			UsageText: `# to login gpud to lepton.ai with an existing, running gpud
sudo gpud login --token <LEPTON_AI_TOKEN>
`,
			Action: cmdlogin.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				cli.StringFlag{
					Name:  "token",
					Usage: "lepton.ai workspace token for checking in",
				},
				cli.StringFlag{
					Name:  "endpoint",
					Usage: "endpoint for control plane",
					Value: "gpud-manager-prod01.dgxc-lepton.nvidia.com",
				},
				cli.StringFlag{
					Name:   "machine-id",
					Hidden: true,
					Usage:  "for override default machine id",
				},
				cli.StringFlag{
					Name:  "node-group",
					Usage: "node group to join",
				},
				cli.StringFlag{
					Name:  "gpu-count",
					Usage: "specify count of gpu",
				},
				cli.StringFlag{
					Name:  "private-ip",
					Usage: "can specify private ip for internal network",
				},
				cli.StringFlag{
					Name:  "public-ip",
					Usage: "can specify public ip for machine",
				},
			},
		},
		{
			Name:  "up",
			Usage: "initialize and start gpud in a daemon mode (systemd)",
			UsageText: `# to start gpud as a systemd unit (recommended)
sudo gpud up

# to enable machine monitoring powered by lepton.ai platform
# sign up here: https://lepton.ai
sudo gpud up --token <LEPTON_AI_TOKEN>

# to start gpud without a systemd unit (e.g., mac)
gpud run

# or
nohup sudo gpud run &>> <your log file path> &
`,
			Action: cmdup.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				cli.StringFlag{
					Name:  "token",
					Usage: "lepton.ai workspace token for checking in",
				},
				cli.StringFlag{
					Name:  "private-ip",
					Usage: "can specify private ip for internal network",
				},
				cli.StringFlag{
					Name:  "public-ip",
					Usage: "can specify public ip for machine",
				},
				cli.StringFlag{
					Name:   "machine-id",
					Hidden: true,
					Usage:  "for override default machine id",
				},
				cli.StringFlag{
					Name:  "endpoint",
					Usage: "endpoint for checking in",
					Value: "gpud-manager-prod01.dgxc-lepton.nvidia.com",
				},
				cli.StringFlag{
					Name:  "gpu-count",
					Usage: "specify count of gpu",
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
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
			},
		},
		{
			Name:   "run",
			Usage:  "starts gpud without any login/checkin ('gpud up' is recommended for linux)",
			Action: cmdrun.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:   "endpoint",
					Usage:  "endpoint for control plane",
					Hidden: true,
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
					Value: fmt.Sprintf("0.0.0.0:%d", config.DefaultGPUdPort),
				},
				&cli.BoolFlag{
					Name:  "pprof",
					Usage: "enable pprof (default: false)",
				},
				&cli.DurationFlag{
					Name:  "retention-period",
					Usage: "set the time period to retain metrics for (once elapsed, old records are compacted/purged)",
					Value: config.DefaultRetentionPeriod.Duration,
				},
				&cli.BoolTFlag{
					Name:  "enable-auto-update",
					Usage: "enable auto update of gpud (default: true)",
				},
				&cli.IntFlag{
					Name:  "auto-update-exit-code",
					Usage: "specifies the exit code to exit with when auto updating (default: -1 to disable exit code)",
					Value: -1,
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

				// only for testing
				cli.StringFlag{
					Name:   "ibstat-command",
					Usage:  "sets the ibstat command (leave empty for default, useful for testing)",
					Value:  "ibstat",
					Hidden: true,
				},
				cli.StringFlag{
					Name:   "ibstatus-command",
					Usage:  "sets the ibstatus command (leave empty for default, useful for testing)",
					Value:  "ibstatus",
					Hidden: true,
				},
			},
		},

		// operations
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

		// for notifying control plane state change
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
							Name:  "log-level,l",
							Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
						},
					},
				},
			},
		},

		// for checking gpud status
		{
			Name:    "status",
			Aliases: []string{"st"},

			Usage:  "checks the status of gpud",
			Action: cmdstatus.Command,
			Flags: []cli.Flag{
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
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
			},
		},

		// for diagnose + quick scanning
		{
			Name:    "scan",
			Aliases: []string{"check", "s"},

			Usage:  "quick scans the host for any major issues",
			Action: cmdscan.CreateCommand(),
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				&cli.StringFlag{
					Name:  "nfs-checker-configs",
					Usage: "set the NFS checker group configs in JSON (leave empty for default, useful for testing)",
				},

				// only for testing
				cli.StringFlag{
					Name:   "ibstat-command",
					Usage:  "sets the ibstat command (leave empty for default, useful for testing)",
					Hidden: true,
				},
				cli.StringFlag{
					Name:   "ibstatus-command",
					Usage:  "sets the ibstatus command (leave empty for default, useful for testing)",
					Hidden: true,
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
			Name:  "join",
			Usage: "join gpud machine into a lepton cluster",
			UsageText: `# to join gpud into a lepton cluster
sudo gpud join
`,
			Action: cmdjoin.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				cli.StringFlag{
					Name:   "cluster-name",
					Usage:  "[DEPRECATED] cluster name for control plane (e.g.: lepton-prod-0)",
					Value:  "",
					Hidden: true,
				},
				cli.StringFlag{
					Name:  "provider",
					Usage: "provider of the machine",
				},
				cli.StringFlag{
					Name:  "provider-instance-id",
					Usage: "provider instance id of the machine",
				},
				cli.StringFlag{
					Name:  "node-group",
					Usage: "node group to join",
				},
				cli.StringFlag{
					Name:  "public-ip",
					Usage: "can specify public ip for machine",
				},
				cli.StringFlag{
					Name:  "private-ip",
					Usage: "can specify private ip for internal network",
				},
				cli.BoolFlag{
					Name:  "skip-interactive",
					Usage: "use detected value instead of prompting for user input",
				},
				cli.StringFlag{
					Name:  "extra-info",
					Usage: "base64 encoded extra info to pass to control plane",
				},
				cli.StringFlag{
					Name:  "region",
					Usage: "specify the region of the machine",
				},
				cli.StringFlag{
					Name:  "gpu-product",
					Usage: "specify the GPU shape of the machine",
				},
			},
		},

		// for diagnose + quick scanning
		{
			Name:    "custom-plugins",
			Aliases: []string{"cs", "plugin", "plugins"},

			Usage:  "checks/runs custom plugins",
			Action: cmdcustomplugins.Command,
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

				// only for testing
				cli.StringFlag{
					Name:   "ibstat-command",
					Usage:  "sets the ibstat command (leave empty for default, useful for testing)",
					Hidden: true,
				},
				cli.StringFlag{
					Name:   "ibstatus-command",
					Usage:  "sets the ibstatus command (leave empty for default, useful for testing)",
					Hidden: true,
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
	}

	return app
}
