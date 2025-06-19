package nfschecker

import (
	"encoding/json"
	"os"
)

type Data struct {
	VolumeName      string `json:"volume_name"`
	VolumeMountPath string `json:"volume_mount_path"`
	FileContents    string `json:"file_contents"`
}

// Write writes the data to the file.
func (d Data) Write(file string) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return os.WriteFile(file, b, 0644)
}

// ReadDataFromFile reads the data from the file.
// It supports both the old and new format.
// The old format is when only the file contents are written to the file.
func ReadDataFromFile(file string) (Data, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return Data{}, err
	}

	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		// old format that only wrote the file contents
		// TODO: fail this once migration is complete
		return Data{FileContents: string(b)}, nil
	}

	return d, nil
}
