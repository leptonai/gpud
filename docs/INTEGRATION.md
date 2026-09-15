# Integration with GPUd

To interact with GPUd API endpoints:

```bash
# healthiness of the GPUd process itself
curl -kL https://localhost:15132/healthz

# basic machine information
curl -kL https://localhost:15132/machine-info | jq | less

# list of health check states
curl -kL https://localhost:15132/v1/states | jq | less

# list of systemd events per GPUd component
# (e.g., xid)
curl -kL https://localhost:15132/v1/events | jq | less

# list of system metrics per GPUd component
# (e.g., GPU temperature)
curl -kL https://localhost:15132/v1/metrics | jq | less
```

Following defines the response types for the GPUd APIs above:

- [API types in Go struct](https://github.com/leptonai/gpud/blob/main/api/v1/types.go)
- [OpenAPI spec in JSON](https://github.com/leptonai/gpud/blob/main/docs/apis/swagger.json)
- [OpenAPI spec in YAML](https://github.com/leptonai/gpud/blob/main/docs/apis/swagger.yaml)

Or use the [`client/v1`](http://pkg.go.dev/github.com/leptonai/gpud/client/v1) library to interact with GPUd in Go.

## Lepton-managed diagnostics

When GPUd is connected to gpud-manager through the session stream, gpud-manager
can send a `method: "diagnostic"` request. GPUd only accepts fixed diagnostic
types. The current Lepton-managed type is:

```text
nvidia_bug_report -> /usr/bin/nvidia-bug-report.sh
```

The session request carries `report_id`, `type`, and `timeout_seconds`; it does
not carry user-provided commands, arguments, scripts, or environment variables.

GPUd runs the fixed diagnostic asynchronously after accepting the request. The
default execution timeout is 10 minutes. If the script exceeds the timeout,
GPUd cancels the process, treats the diagnostic as failed, and notifies
gpud-manager through:

```http
POST /api/v1/diagnostics/{report_id}/failure
```

GPUd also uses the failure endpoint when the script produces no artifact, gzip
conversion fails, or report upload fails after retries. If the command exits
nonzero but still produces a report, GPUd uploads the report so operators can
inspect it. Report uploads and failure notifications use the machine session
token, the machine id headers, and the control-plane origin header. GPUd must
not log report contents, captured script output, storage URLs, or tokens.

## Lepton-managed KAP credentials

The KAP credential session commands manage validated credential files and client
certificate reloads, not the agent service lifecycle. The package controller and
the `kap-mtls-agent` package's `init.sh` own installation, startup and recovery.

- Initial credentials can be staged while the agent is inactive. The update
  returns without waiting for startup or holding the package lock across it.
- Ordinary renewals preserve the existing CA and gateway configuration, select a
  complete immutable credential generation atomically, and signal a running
  agent with SIGHUP. The certificate metrics confirm the loaded serial and
  expiry only; they do not prove which startup configuration the process uses.
- Failed reload acknowledgement preserves the staged generation for retry;
  the credential handler does not restart the service or roll back activation.
- A different credential generation must have a distinct leaf serial or expiry;
  otherwise those metrics cannot distinguish it from the previous leaf.
- The existing `activateKAPMTLS` command remains a compatibility probe. It does
  not start or restart the service. An inactive, unready or unconfirmed agent
  reports an error while the package controller handles recovery.

Changing the gateway CA, client-CA fingerprint, gateway endpoint or server name
requires a separate maintenance procedure with an explicit agent restart. The
renewal command rejects these changes before writing files, even while the
agent is inactive. Conflicting or corrupt retained configuration also requires
explicit maintenance rather than inferring activation from certificate metrics.
Do not use readiness as proof that a changed endpoint or trust pool is active.
