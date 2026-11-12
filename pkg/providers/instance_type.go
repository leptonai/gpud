package providers

import "context"

// InstanceTypeDetector provides the provider's actual machine type, when supported.
type InstanceTypeDetector interface {
	InstanceType(context.Context) (string, error)
}

// WithInstanceType enables optional machine-type collection without changing
// the identity fields required for enrollment.
func WithInstanceType(fetch func(context.Context) (string, error)) DetectorOption {
	return func(d *detector) { d.fetchInstanceTypeFunc = fetch }
}

func (d *detector) InstanceType(ctx context.Context) (string, error) {
	if d.fetchInstanceTypeFunc == nil {
		return "", nil
	}
	return d.fetchInstanceTypeFunc(ctx)
}

func (d *detector) supportsInstanceType() bool { return d.fetchInstanceTypeFunc != nil }

// SupportsInstanceType reports whether a detector has a machine-type reader.
func SupportsInstanceType(d Detector) bool {
	reader, ok := d.(interface{ supportsInstanceType() bool })
	return ok && reader.supportsInstanceType()
}
