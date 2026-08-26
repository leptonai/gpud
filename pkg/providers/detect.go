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

type detector struct {
	providerName           string
	detectProviderFunc     func(ctx context.Context) (string, error)
	fetchPublicIPv4Func    func(ctx context.Context) (string, error)
	fetchPrivateIPv4Func   func(ctx context.Context) (string, error)
	fetchVMEnvironmentFunc func(ctx context.Context) (string, error)
	fetchInstanceIDFunc    func(ctx context.Context) (string, error)
	imds                   bool
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
) Detector {
	d := &regionDetector{
		detector:        newDetector(name, detectProviderFunc, fetchPublicIPv4Func, fetchPrivateIPv4Func, fetchVMEnvironmentFunc, fetchInstanceIDFunc),
		fetchRegionFunc: fetchRegionFunc,
	}
	d.imds = true
	return d
}

func SupportsIMDS(d Detector) bool {
	imds, ok := d.(imdsDetector)
	return ok && imds.supportsIMDS()
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
