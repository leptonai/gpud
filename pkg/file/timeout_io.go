package file

import (
	"context"
	"os"
)

// StatWithTimeout performs os.Stat with timeout from context to prevent blocking on unresponsive filesystems like NFS
func StatWithTimeout(ctx context.Context, path string) (os.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	type result struct {
		info os.FileInfo
		err  error
	}
	resultCh := make(chan result, 1)

	go func() {
		info, err := os.Stat(path)
		select {
		case resultCh <- result{info: info, err: err}:
		case <-ctx.Done():
		}
	}()

	select {
	case res := <-resultCh:
		return res.info, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// MkdirAllWithTimeout performs os.MkdirAll with timeout from context to prevent blocking on unresponsive filesystems like NFS
func MkdirAllWithTimeout(ctx context.Context, path string, perm os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	errCh := make(chan error, 1)

	go func() {
		err := os.MkdirAll(path, perm)
		select {
		case errCh <- err:
		case <-ctx.Done():
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WriteFileWithTimeout performs os.WriteFile with timeout from context to prevent blocking on unresponsive filesystems like NFS
func WriteFileWithTimeout(ctx context.Context, name string, data []byte, perm os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	errCh := make(chan error, 1)

	go func() {
		err := os.WriteFile(name, data, perm)
		select {
		case errCh <- err:
		case <-ctx.Done():
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReadFileWithTimeout performs os.ReadFile with timeout from context to prevent blocking on unresponsive filesystems like NFS
func ReadFileWithTimeout(ctx context.Context, name string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	type result struct {
		data []byte
		err  error
	}
	resultCh := make(chan result, 1)

	go func() {
		data, err := os.ReadFile(name)
		select {
		case resultCh <- result{data: data, err: err}:
		case <-ctx.Done():
		}
	}()

	select {
	case res := <-resultCh:
		return res.data, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
