package file

import (
	"context"
	"os"
	"testing"
	"time"

	query_config "github.com/leptonai/gpud/components/query/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComponent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create test files
	testFile1 := "/tmp/test.txt"
	testFile2 := "/tmp/1231232131.txt"

	// create the first test file
	if err := os.WriteFile(testFile1, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	defer os.Remove(testFile1)

	component := New(
		ctx,
		Config{
			Query: query_config.Config{
				Interval: metav1.Duration{Duration: 5 * time.Second},
			},
			Files: []File{
				{
					Path:          testFile1,
					RequireExists: true,
				},
				{
					Path:          testFile2,
					RequireExists: true,
				},
			},
		},
	)

	time.Sleep(time.Second)

	states, err := component.States(ctx)
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	t.Logf("states: %+v", states)
	if len(states) != 1 {
		t.Fatalf("expected 1 states, got %d", len(states))
	}
	if states[0].Healthy {
		t.Fatalf("expected unhealthy state, got healthy")
	}

	parsedOutput, err := ParseStatesToOutput(states...)
	if err != nil {
		t.Fatalf("failed to parse states: %v", err)
	}
	t.Logf("parsed output: %+v", parsedOutput)
}
