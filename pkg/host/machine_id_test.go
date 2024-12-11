package host

import (
	"bufio"
	"os"
	"testing"
)

func TestScanUUIDFromDmidecode(t *testing.T) {
	f, err := os.Open("testdata/dmidecode")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	uuid := scanUUIDFromDmidecode(scanner)
	if uuid != "4c4c4544-0053-5210-8038-c8c04f583034" {
		t.Errorf("expected UUID to be 4c4c4544-0053-5210-8038-c8c04f583034, got %s", uuid)
	}
}
