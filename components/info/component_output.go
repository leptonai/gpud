package info

import "encoding/json"

type Output struct {
	DaemonVersion string `json:"daemon_version"`
	MacAddress    string `json:"mac_address"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

func ParseOutputJSON(data []byte) (*Output, error) {
	o := new(Output)
	if err := json.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}
