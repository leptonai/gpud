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

## Register with a workspace

Workspace-aware registration requires a control plane with `POST /api/v2/login`
support. Use the canonical workspace name, not a workspace display name or wildcard:

```bash
sudo gpud up \
  --token <REGISTRATION_TOKEN> \
  --endpoint <CONTROL_PLANE_ENDPOINT> \
  --workspace-id <CANONICAL_WORKSPACE_NAME> \
  --node-group <NODE_GROUP>
```

`--workspace-id` requires a nonempty `--token` and `--node-group`.
Do not combine it with `--machine-id`; neither selector accepts `*`.
GPUd validates these inputs before root checks, discovery, or systemd changes.
Workspace-aware registration does not fall back to legacy login if the server
rejects the request or lacks v2 support. Omit `--workspace-id` for legacy enrollment.

This flag applies only to new registration through `gpud up`, not `gpud run` or
workspace migration. An already-registered machine normally reuses its persisted
identity without login. A forced session refresh uses `/api/v1/login` with that
identity and its machine proof, without the workspace selector.
Neither the registration token nor workspace selector enters the systemd environment.

See [the integration contract](INTEGRATION.md#workspace-aware-registration) for
request routing and server requirements.

## Instance IDs without a metadata service

If provider metadata does not supply an instance ID, GPUd reads the host's
cloud-init files. GPUd treats instance IDs as opaque strings and uses
`strings.TrimSpace` to remove surrounding whitespace.
Recognized UUIDs and Firebird canonical node IDs retain their existing case normalization.
GPUd rejects empty values and these cloud-init placeholders after trimming:
`iid-datasource-none`, `iid-datasource`, `nocloud`, `iid-dsconfigdrive`,
`iid-dsovf`, and `iid-dsopennebula`.
It skips those files and uses the first valid ID from the remaining fallback files.

For Linode, GPUd reads `/var/lib/cloud/data/instance-id` in a systemd installation.
In a DaemonSet, the host file is available at `/host/var/lib/cloud/data/instance-id`
when `gpud.mountHostRoot` is enabled.
The login request uses the decimal ID without Kubernetes's `linode://` prefix.
An instance ID from provider metadata takes precedence over the host file.
NScale does not use this fallback because its cloud-init UUID differs from
its API instance ID.

If the host has no usable instance ID, provide the provider's canonical ID during enrollment:

```bash
sudo gpud up \
  --token <REGISTRATION_TOKEN> \
  --endpoint <CONTROL_PLANE_ENDPOINT> \
  --node-group <NODE_GROUP> \
  --instance-id <PROVIDER_INSTANCE_ID>
```

`--instance-id` defaults to an empty string. A nonempty value is sent unchanged as
`providerInstanceID` in the login request and skips instance-ID discovery, including
provider metadata and host-file fallbacks. An empty or omitted value keeps normal discovery.
Provider, region, and network discovery still run; use `--region` separately if needed.

The override applies only when login runs. It is not stored in local metadata or
the systemd environment and does not change later machine-info collection.
It does not replace `--machine-id`, which identifies the control-plane Machine record.

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
