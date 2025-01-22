package nvml

import "os"

const (
	EnvNVMLMock = "GPUD_MOCK_NVML"
)

func IsMockedNVML() bool {
	return os.Getenv(EnvNVMLMock) == "true"
}
