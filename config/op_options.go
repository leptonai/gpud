package config

type Op struct {
	FilesToCheck                  []string
	KernelModulesToCheck          []string
	DockerIgnoreConnectionErrors  bool
	KubeletIgnoreConnectionErrors bool
}

type OpOption func(*Op)

func (op *Op) ApplyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	return nil
}

func WithFilesToCheck(files ...string) OpOption {
	return func(op *Op) {
		op.FilesToCheck = append(op.FilesToCheck, files...)
	}
}

func WithKernelModulesToCheck(modules ...string) OpOption {
	return func(op *Op) {
		op.KernelModulesToCheck = append(op.KernelModulesToCheck, modules...)
	}
}

func WithDockerIgnoreConnectionErrors(b bool) OpOption {
	return func(op *Op) {
		op.DockerIgnoreConnectionErrors = b
	}
}

func WithKubeletIgnoreConnectionErrors(b bool) OpOption {
	return func(op *Op) {
		op.KubeletIgnoreConnectionErrors = b
	}
}
