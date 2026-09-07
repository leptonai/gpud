package providers

import (
	"context"
)

var (
	_ Detector       = &detector{}
	_ Detector       = &regionDetector{}
	_ RegionDetector = &regionDetector{}
)

type imdsDetector interface {
	supportsIMDS() bool
}

type privateIPv4RetryDetector interface {
	retriesPrivateIPv4() bool
}

type detector struct {
	providerName           string
	detectProviderFunc     func(ctx context.Context) (string, error)
	fetchPublicIPv4Func    func(ctx context.Context) (string, error)
	fetchPrivateIPv4Func   func(ctx context.Context) (string, error)
	fetchVMEnvironmentFunc func(ctx context.Context) (string, error)
	fetchInstanceIDFunc    func(ctx context.Context) (string, error)
	imds                   bool
	retryPrivateIPv4       bool
}

type regionDetector struct {
	*detector
	fetchRegionFunc func(ctx context.Context) (string, error)
}

func New(
	name string,
	detectProviderFunc func(ctx context.Context) (string, error),
	fetchPublicIPv4Func func(ctx context.Context) (string, error),
	fetchPrivateIPv4Func func(ctx context.Context) (string, error),
	fetchVMEnvironmentFunc func(ctx context.Context) (string, error),
	fetchInstanceIDFunc func(ctx context.Context) (string, error),
) Detector {
	return newDetector(name, detectProviderFunc, fetchPublicIPv4Func, fetchPrivateIPv4Func, fetchVMEnvironmentFunc, fetchInstanceIDFunc)
}

func NewWithRegion(
	name string,
	detectProviderFunc func(ctx context.Context) (string, error),
	fetchPublicIPv4Func func(ctx context.Context) (string, error),
	fetchPrivateIPv4Func func(ctx context.Context) (string, error),
	fetchRegionFunc func(ctx context.Context) (string, error),
	fetchVMEnvironmentFunc func(ctx context.Context) (string, error),
	fetchInstanceIDFunc func(ctx context.Context) (string, error),
) Detector {
	return &regionDetector{
		detector:        newDetector(name, detectProviderFunc, fetchPublicIPv4Func, fetchPrivateIPv4Func, fetchVMEnvironmentFunc, fetchInstanceIDFunc),
		fetchRegionFunc: fetchRegionFunc,
	}
}

func NewIMDSWithRegion(
	name string,
	detectProviderFunc func(ctx context.Context) (string, error),
	fetchPublicIPv4Func func(ctx context.Context) (string, error),
	fetchPrivateIPv4Func func(ctx context.Context) (string, error),
	fetchRegionFunc func(ctx context.Context) (string, error),
	fetchVMEnvironmentFunc func(ctx context.Context) (string, error),
	fetchInstanceIDFunc func(ctx context.Context) (string, error),
	opts ...DetectorOption,
) Detector {
	d := &regionDetector{
		detector:        newDetector(name, detectProviderFunc, fetchPublicIPv4Func, fetchPrivateIPv4Func, fetchVMEnvironmentFunc, fetchInstanceIDFunc),
		fetchRegionFunc: fetchRegionFunc,
	}
	d.imds = true
	for _, opt := range opts {
		opt(d.detector)
	}
	return d
}

func SupportsIMDS(d Detector) bool {
	imds, ok := d.(imdsDetector)
	return ok && imds.supportsIMDS()
}

// SupportsPrivateIPv4Retry returns true if the detector opted into retrying
// the private IPv4 fetch (see WithPrivateIPv4Retry).
func SupportsPrivateIPv4Retry(d Detector) bool {
	rd, ok := d.(privateIPv4RetryDetector)
	return ok && rd.retriesPrivateIPv4()
}

// DetectorOption customizes an IMDS-backed detector.
type DetectorOption func(*detector)

// WithPrivateIPv4Retry opts the detector into retry-with-backoff for the
// private IPv4 metadata fetch when it fails or returns empty. Use it only
// for providers whose metadata service is known to be slow to accept the
// first connection after an idle period (e.g., nscale); detectors without
// the option keep single-attempt behavior.
func WithPrivateIPv4Retry() DetectorOption {
	return func(d *detector) {
		d.retryPrivateIPv4 = true
	}
}

func newDetector(
	name string,
	detectProviderFunc func(ctx context.Context) (string, error),
	fetchPublicIPv4Func func(ctx context.Context) (string, error),
	fetchPrivateIPv4Func func(ctx context.Context) (string, error),
	fetchVMEnvironmentFunc func(ctx context.Context) (string, error),
	fetchInstanceIDFunc func(ctx context.Context) (string, error),
) *detector {
	return &detector{
		providerName:           name,
		detectProviderFunc:     detectProviderFunc,
		fetchPublicIPv4Func:    fetchPublicIPv4Func,
		fetchPrivateIPv4Func:   fetchPrivateIPv4Func,
		fetchVMEnvironmentFunc: fetchVMEnvironmentFunc,
		fetchInstanceIDFunc:    fetchInstanceIDFunc,
	}
}

func (d *detector) Name() string {
	return d.providerName
}

func (d *detector) supportsIMDS() bool {
	return d.imds
}

func (d *detector) retriesPrivateIPv4() bool {
	return d.retryPrivateIPv4
}

func (d *detector) Provider(ctx context.Context) (string, error) {
	if d.detectProviderFunc != nil {
		detectedProvider, err := d.detectProviderFunc(ctx)
		if err != nil {
			return "", err
		}
		if detectedProvider != "" {
			return d.providerName, nil
		}
	}

	return "", nil
}

func (d *detector) PublicIPv4(ctx context.Context) (string, error) {
	if d.fetchPublicIPv4Func != nil {
		return d.fetchPublicIPv4Func(ctx)
	}
	return "", nil
}

func (d *detector) PrivateIPv4(ctx context.Context) (string, error) {
	if d.fetchPrivateIPv4Func != nil {
		return d.fetchPrivateIPv4Func(ctx)
	}
	return "", nil
}

func (d *regionDetector) Region(ctx context.Context) (string, error) {
	if d.fetchRegionFunc != nil {
		return d.fetchRegionFunc(ctx)
	}
	return "", nil
}

func (d *detector) VMEnvironment(ctx context.Context) (string, error) {
	if d.fetchVMEnvironmentFunc != nil {
		return d.fetchVMEnvironmentFunc(ctx)
	}
	return "", nil
}

func (d *detector) InstanceID(ctx context.Context) (string, error) {
	if d.fetchInstanceIDFunc != nil {
		return d.fetchInstanceIDFunc(ctx)
	}
	return "", nil
}
