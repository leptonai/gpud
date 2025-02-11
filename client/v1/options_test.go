package v1

import (
	"net/http"
	"testing"
	"time"

	"github.com/leptonai/gpud/internal/server"
)

func TestWithHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: time.Hour}
	op := &Op{}
	opt := WithHTTPClient(customClient)
	opt(op)

	if op.httpClient != customClient {
		t.Errorf("WithHTTPClient() did not set the expected client")
	}
}

func TestWithCheckInterval(t *testing.T) {
	interval := 5 * time.Second
	op := &Op{}
	opt := WithCheckInterval(interval)
	opt(op)

	if op.checkInterval != interval {
		t.Errorf("WithCheckInterval() = %v, want %v", op.checkInterval, interval)
	}
}

func TestWithRequestContentTypeYAML(t *testing.T) {
	op := &Op{}
	opt := WithRequestContentTypeYAML()
	opt(op)

	if op.requestContentType != server.RequestHeaderYAML {
		t.Errorf("WithRequestContentTypeYAML() = %v, want %v", op.requestContentType, server.RequestHeaderYAML)
	}
}

func TestWithRequestContentTypeJSON(t *testing.T) {
	op := &Op{}
	opt := WithRequestContentTypeJSON()
	opt(op)

	if op.requestContentType != server.RequestHeaderJSON {
		t.Errorf("WithRequestContentTypeJSON() = %v, want %v", op.requestContentType, server.RequestHeaderJSON)
	}
}

func TestWithAcceptEncodingGzip(t *testing.T) {
	op := &Op{}
	opt := WithAcceptEncodingGzip()
	opt(op)

	if op.requestAcceptEncoding != server.RequestHeaderEncodingGzip {
		t.Errorf("WithAcceptEncodingGzip() = %v, want %v", op.requestAcceptEncoding, server.RequestHeaderEncodingGzip)
	}
}

func TestWithComponent(t *testing.T) {
	tests := []struct {
		name      string
		component string
	}{
		{
			name:      "single component",
			component: "test-component",
		},
		{
			name:      "empty component name",
			component: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			opt := WithComponent(tt.component)
			opt(op)

			if op.components == nil {
				t.Fatal("WithComponent() did not initialize components map")
			}

			if _, exists := op.components[tt.component]; !exists {
				t.Errorf("WithComponent() did not add component %q to map", tt.component)
			}
		})
	}

	// Test multiple components
	t.Run("multiple components", func(t *testing.T) {
		op := &Op{}
		components := []string{"comp1", "comp2", "comp3"}

		for _, comp := range components {
			WithComponent(comp)(op)
		}

		if len(op.components) != len(components) {
			t.Errorf("WithComponent() expected %d components, got %d", len(components), len(op.components))
		}

		for _, comp := range components {
			if _, exists := op.components[comp]; !exists {
				t.Errorf("WithComponent() component %q not found in map", comp)
			}
		}
	})
}

func TestOp_applyOpts(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts(nil)
		if err != nil {
			t.Errorf("applyOpts() unexpected error = %v", err)
		}

		// Check default http client
		if op.httpClient == nil {
			t.Error("applyOpts() did not set default http client")
		}
		transport, ok := op.httpClient.Transport.(*http.Transport)
		if !ok {
			t.Error("applyOpts() did not set expected transport type")
		}
		if transport.TLSClientConfig.InsecureSkipVerify != true {
			t.Error("applyOpts() did not set InsecureSkipVerify to true")
		}

		// Check default check interval
		if op.checkInterval != time.Second {
			t.Errorf("applyOpts() check interval = %v, want %v", op.checkInterval, time.Second)
		}
	})

	t.Run("custom values", func(t *testing.T) {
		customClient := &http.Client{Timeout: time.Hour}
		customInterval := 5 * time.Second

		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithHTTPClient(customClient),
			WithCheckInterval(customInterval),
			WithRequestContentTypeJSON(),
			WithAcceptEncodingGzip(),
			WithComponent("test"),
		})

		if err != nil {
			t.Errorf("applyOpts() unexpected error = %v", err)
		}

		// Verify all options were applied
		if op.httpClient != customClient {
			t.Error("applyOpts() did not set custom http client")
		}
		if op.checkInterval != customInterval {
			t.Error("applyOpts() did not set custom check interval")
		}
		if op.requestContentType != server.RequestHeaderJSON {
			t.Error("applyOpts() did not set JSON content type")
		}
		if op.requestAcceptEncoding != server.RequestHeaderEncodingGzip {
			t.Error("applyOpts() did not set gzip encoding")
		}
		if _, exists := op.components["test"]; !exists {
			t.Error("applyOpts() did not set component")
		}
	})

	t.Run("multiple applications", func(t *testing.T) {
		op := &Op{}
		// Apply options multiple times to ensure last one wins
		err := op.applyOpts([]OpOption{
			WithCheckInterval(time.Second),
			WithCheckInterval(2 * time.Second),
			WithCheckInterval(3 * time.Second),
		})

		if err != nil {
			t.Errorf("applyOpts() unexpected error = %v", err)
		}

		if op.checkInterval != 3*time.Second {
			t.Errorf("applyOpts() check interval = %v, want %v", op.checkInterval, 3*time.Second)
		}
	})
}
