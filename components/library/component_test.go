package library

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/file"
)

// mockComponent creates a test component with a mocked FindLibrary function
func mockComponent(findLibrary func(string, ...file.OpOption) (string, error), cfg Config) components.Component {
	c := New(cfg).(*component)
	c.findLibrary = findLibrary
	return c
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "empty config",
			cfg:  Config{},
		},
		{
			name: "with libraries and search dirs",
			cfg: Config{
				Libraries: map[string][]string{
					"lib1": {"alt1", "alt2"},
					"lib2": {},
				},
				SearchDirs: []string{"/usr/lib", "/lib"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.cfg)
			require.NotNil(t, c)
			assert.Equal(t, Name, c.Name())
		})
	}
}

func TestStates(t *testing.T) {
	tests := []struct {
		name          string
		cfg           Config
		mockLibFinder func(lib string, opts ...file.OpOption) (string, error)
		wantHealthy   bool
		wantReason    string
		wantErr       bool
	}{
		{
			name: "all libraries exist",
			cfg: Config{
				Libraries: map[string][]string{
					"lib1": {"alt1"},
					"lib2": {},
				},
				SearchDirs: []string{"/usr/lib"},
			},
			mockLibFinder: func(lib string, opts ...file.OpOption) (string, error) {
				return "/usr/lib/" + lib, nil
			},
			wantHealthy: true,
			wantReason:  "all libraries exist",
		},
		{
			name: "missing libraries",
			cfg: Config{
				Libraries: map[string][]string{
					"lib1": {"alt1"},
					"lib2": {},
					"lib3": {},
				},
			},
			mockLibFinder: func(lib string, opts ...file.OpOption) (string, error) {
				return "", file.ErrLibraryNotFound
			},
			wantHealthy: false,
			wantReason:  `library "lib1" does not exist; library "lib2" does not exist; library "lib3" does not exist`,
		},
		{
			name: "error finding library",
			cfg: Config{
				Libraries: map[string][]string{
					"lib1": {},
				},
			},
			mockLibFinder: func(lib string, opts ...file.OpOption) (string, error) {
				return "", assert.AnError
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mockComponent(tt.mockLibFinder, tt.cfg)
			states, err := c.States(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Len(t, states, 1)
			assert.Equal(t, Name, states[0].Name)
			assert.Equal(t, tt.wantHealthy, states[0].Healthy)
			assert.Equal(t, tt.wantReason, states[0].Reason)
		})
	}
}

func TestEvents(t *testing.T) {
	c := New(Config{})
	events, err := c.Events(context.Background(), time.Time{})
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestMetrics(t *testing.T) {
	c := New(Config{})
	metrics, err := c.Metrics(context.Background(), time.Time{})
	assert.NoError(t, err)
	assert.Empty(t, metrics)
}

func TestClose(t *testing.T) {
	c := New(Config{})
	assert.NoError(t, c.Close())
}

func TestStart(t *testing.T) {
	c := New(Config{})
	assert.NoError(t, c.Start())
}
