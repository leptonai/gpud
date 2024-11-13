package common

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

type Filter struct {
	Name string `json:"name"`

	Substring *string `json:"substring,omitempty"`

	Regex *string        `json:"regex,omitempty"`
	regex *regexp.Regexp `json:"-"`

	// OwnerReferences is a list of component names that watches on this filter.
	// Useful when multiple components watch on the same log file.
	// e.g., if the component X and Y both watch on the same log file,
	// with the same filter rule (substring/regex), this field will be
	// set to [x, y].
	OwnerReferences []string `json:"owner_references,omitempty"`
}

func (f *Filter) JSON() ([]byte, error) {
	return json.Marshal(f)
}

func ParseFilterJSON(data []byte) (*Filter, error) {
	f := new(Filter)
	if err := json.Unmarshal(data, f); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *Filter) YAML() ([]byte, error) {
	return yaml.Marshal(f)
}

func ParseFilterYAML(data []byte) (*Filter, error) {
	f := new(Filter)
	err := yaml.Unmarshal(data, f)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Compiles the regex, if set.
func (f *Filter) Compile() error {
	if f.Regex != nil {
		rgx, err := regexp.Compile(*f.Regex)
		if err != nil {
			return err
		}
		f.regex = rgx
	}
	return nil
}

func (f *Filter) MatchString(line string) (bool, error) {
	if f.Regex != nil && f.regex == nil {
		if err := f.Compile(); err != nil {
			return false, err
		}
	}
	return f.matchString(line), nil
}

func (f *Filter) MatchBytes(line []byte) (bool, error) {
	if f.Regex != nil && f.regex == nil {
		if err := f.Compile(); err != nil {
			return false, err
		}
	}
	return f.matchBytes(line), nil
}

func (f *Filter) matchString(line string) bool {
	if f.Substring != nil && strings.Contains(line, *f.Substring) {
		return true
	}
	if f.regex != nil && f.regex.MatchString(line) {
		return true
	}
	return false
}

func (f *Filter) matchBytes(line []byte) bool {
	if f.Substring != nil && bytes.Contains(line, []byte(*f.Substring)) {
		return true
	}
	if f.regex != nil && f.regex.Match(line) {
		return true
	}
	return false
}
