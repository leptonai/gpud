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
