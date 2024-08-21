package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/leptonai/gpud/components"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"

	"github.com/dustin/go-humanize"
)

type Output struct {
	Files []File `json:"files"`
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

const (
	StateNameFile = "file"

	StateNameFileData          = "data"
	StateNameFileEncoding      = "encoding"
	StateValueFileEncodingJSON = "json"
)

func ParseStateFile(m map[string]string) (*Output, error) {
	o := &Output{}
	data := m[StateNameFileData]
	if err := json.Unmarshal([]byte(data), o); err != nil {
		return nil, err
	}
	return o, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameFile:
			return ParseStateFile(state.ExtraInfo)

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return nil, fmt.Errorf("no state found")
}

func (o *Output) States() ([]components.State, error) {
	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameFile,
		Healthy: true,
		Reason:  fmt.Sprintf("checked %d files", len(o.Files)),
		ExtraInfo: map[string]string{
			StateNameFileData:     string(b),
			StateNameFileEncoding: StateValueFileEncodingJSON,
		},
	}

	errs := []string{}
	for _, file := range o.Files {
		if file.RequireExists && !file.Exists {
			errs = append(errs, fmt.Sprintf("file %s does not exist", file.Path))
		}
	}
	if len(errs) > 0 {
		state.Healthy = false
		state.Reason += fmt.Sprintf("-- %s", strings.Join(errs, ", "))
	}

	return []components.State{state}, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(Name, cfg.Query, CreateGet(cfg))
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func CreateGet(cfg Config) query.GetFunc {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(Name)
			} else {
				components_metrics.SetGetSuccess(Name)
			}
		}()

		o := &Output{}
		for _, file := range cfg.Files {
			f, err := checkFile(file.Path, file.RequireExists)
			if err != nil {
				return nil, err
			}
			o.Files = append(o.Files, f)
		}

		return o, nil
	}
}

func checkFile(path string, requireExists bool) (File, error) {
	f := File{
		Path:          path,
		RequireExists: requireExists,
	}

	stat, err := os.Stat(path)
	f.Exists = err == nil

	if err != nil && !os.IsNotExist(err) {
		return f, err
	}

	if f.Exists {
		f.Size = stat.Size()
		f.SizeHumanized = humanize.Bytes(uint64(f.Size))
	}

	return f, nil
}
