//go:build !linux

package disk

import "context"

func probeNativeNFSUsage(ctx context.Context, part Partition) (*Usage, error) {
	return GetUsage(ctx, part.MountPoint)
}
