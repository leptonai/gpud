package informer

type Informer interface {
	Start(<-chan struct{})
}
