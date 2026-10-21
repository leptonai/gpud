package httputil

const (
	RequestHeaderContentType = "Content-Type"
	RequestHeaderJSON        = "application/json"
	RequestHeaderYAML        = "application/yaml"
	RequestHeaderJSONIndent  = "json-indent"

	RequestHeaderAcceptEncoding = "Accept-Encoding"
	RequestHeaderEncodingGzip   = "gzip"

	// RequestHeaderMachineProof carries the machine proof on login and session
	// requests.
	RequestHeaderMachineProof = "X-GPUD-Machine-Proof"
)
