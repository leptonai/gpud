package providers

import (
	"context"
)

var _ Detector = &detector{}

type detector struct {
	providerName           string
	detectProviderFunc     func(ctx context.Context) (string, error)
	fetchPublicIPv4Func    func(ctx context.Context) (string, error)
	fetchVMEnvironmentFunc func(ctx context.Context) (string, error)
	fetchInstanceIDFunc    func(ctx context.Context) (string, error)
}

func New(
	name string,
	detectProviderFunc func(ctx context.Context) (string, error),
	fetchPublicIPv4Func func(ctx context.Context) (string, error),
	fetchVMEnvironmentFunc func(ctx context.Context) (string, error),
	fetchInstanceIDFunc func(ctx context.Context) (string, error),
) Detector {
	return &detector{
		providerName:           name,
		detectProviderFunc:     detectProviderFunc,
		fetchPublicIPv4Func:    fetchPublicIPv4Func,
		fetchVMEnvironmentFunc: fetchVMEnvironmentFunc,
		fetchInstanceIDFunc:    fetchInstanceIDFunc,
	}
}

func (d *detector) Name() string {
	return d.providerName
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
