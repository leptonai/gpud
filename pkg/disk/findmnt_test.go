package disk

import (
	"os"
	"testing"
)

func TestParseFindMntOutput(t *testing.T) {
	for _, file := range []string{"findmnt.0.json", "findmnt.1.json"} {
		b, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatalf("error reading test data: %v", err)
		}
		output, err := ParseFindMntOutput(string(b))
		if err != nil {
			t.Fatalf("error finding mount target output: %v", err)
		}
		t.Logf("output: %+v", output)
	}
}
