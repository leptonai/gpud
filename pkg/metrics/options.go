package metrics

import "time"

type Op struct {
	Since              time.Time
	SelectedComponents map[string]struct{}
}

type OpOption func(*Op)

func (op *Op) ApplyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	return nil
}

func WithSince(t time.Time) OpOption {
	return func(op *Op) {
		op.Since = t
	}
}

// WithComponents sets the components to be scraped.
// If no components are provided, all components will be scraped.
func WithComponents(components ...string) OpOption {
	return func(op *Op) {
		for _, component := range components {
			if len(component) == 0 {
				continue
			}

			if op.SelectedComponents == nil {
				op.SelectedComponents = make(map[string]struct{})
			}
			op.SelectedComponents[component] = struct{}{}
		}
	}
}
