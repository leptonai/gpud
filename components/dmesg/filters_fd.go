package dmesg

import (
	fd_dmesg "github.com/leptonai/gpud/components/fd/dmesg"
	fd_id "github.com/leptonai/gpud/components/fd/id"
	query_log_common "github.com/leptonai/gpud/internal/query/log/common"

	"k8s.io/utils/ptr"
)

const (
	// e.g.,
	// [...] VFS: file-max limit 1000000 reached
	//
	// ref.
	// https://docs.kernel.org/admin-guide/sysctl/fs.html#file-max-file-nr
	EventFileDescriptorVFSFileMaxLimitReached = "file_descriptor_vfs_file_max_limit_reached"
)

func DefaultDmesgFiltersForFileDescriptor() []*query_log_common.Filter {
	return []*query_log_common.Filter{
		{
			Name:            EventFileDescriptorVFSFileMaxLimitReached,
			Regex:           ptr.To(fd_dmesg.RegexVFSFileMaxLimitReached),
			OwnerReferences: []string{fd_id.Name},
		},
	}
}
