package query

import "testing"

func Test_parseFabricManagerVersion(t *testing.T) {
	t.Parallel()

	input := `


	Fabric Manager version is : 535.161.08

`

	ver := parseFabricManagerVersion(input)
	if ver != "535.161.08" {
		t.Errorf("Expected 535.161.08, but got: %s", ver)
	}
}
