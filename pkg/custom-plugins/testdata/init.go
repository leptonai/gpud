package testdata

import (
	_ "embed"

	customplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"sigs.k8s.io/yaml"
)

//go:embed plugins.plaintext.2.regex.yaml
var exampleSpecsYAML []byte

var exampleSpecs customplugins.Specs

func init() {
	var err error
	err = yaml.Unmarshal(exampleSpecsYAML, &exampleSpecs)
	if err != nil {
		panic(err)
	}
	if err = exampleSpecs.Validate(); err != nil {
		panic(err)
	}
}

func ExampleSpecs() customplugins.Specs {
	return exampleSpecs
}
