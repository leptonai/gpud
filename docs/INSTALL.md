# Install GPUd

To install from the official release:

```bash
curl -fsSL https://pkg.gpud.dev/install.sh | sh
gpud up
```

To install the latest published version explicitly:

```bash
curl -fsSL https://pkg.gpud.dev/install.sh | sh -s $(curl -fsSL https://pkg.gpud.dev/unstable_latest.txt)
```

Then open [localhost:15132](https://localhost:15132) for the local web UI.

## Instance IDs without a metadata service

If provider metadata does not supply an instance ID, GPUd reads the host's cloud-init files.
It accepts UUIDs, Firebird canonical node IDs, and decimal IDs such as Linode's `108254612`.
GPUd removes surrounding whitespace, including the newline in cloud-init's `instance-id` file.
It rejects empty values and the `iid-datasource-none` placeholder.

For Linode, GPUd reads `/var/lib/cloud/data/instance-id` in a systemd installation.
In a DaemonSet, the host file is available at `/host/var/lib/cloud/data/instance-id`
when `gpud.mountHostRoot` is enabled.
The login request uses the decimal ID without Kubernetes's `linode://` prefix.
An instance ID from provider metadata takes precedence over the host file.
NScale does not use this fallback because its cloud-init UUID differs from its API instance ID.

## Build

To build and run locally:

```bash
make all
./bin/gpud up
```

To run without systemd (e.g., Mac OS):

```bash
./bin/gpud run
```
