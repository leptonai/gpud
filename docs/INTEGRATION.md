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
