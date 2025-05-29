package xid

import "fmt"

type MessageToInject struct {
	Priority string
	Message  string
}

func GetMessageToInject(xid int) MessageToInject {
	msg, ok := xidExampleMsgs[xid]
	if !ok {
		return MessageToInject{
			Priority: "KERN_WARNING",
			Message:  fmt.Sprintf("NVRM: Xid (PCI:0000:04:00): %d, unknown", xid),
		}
	}
	return msg
}

var xidExampleMsgs = map[int]MessageToInject{
	63: {
		Priority: "KERN_WARNING",
		Message:  "NVRM: Xid (PCI:0000:04:00): 63, Row remapping event: Rows 0x1a and 0x2b have been remapped on GPU 00000000:04:00.0",
	},
	64: {
		Priority: "KERN_WARNING",
		Message:  "NVRM: Xid (PCI:0000:04:00): 64, Failed to persist row remap table — GPU may require servicing",
	},
	69: {
		Priority: "KERN_WARNING",
		Message:  "NVRM: Xid (PCI:0000:04:00): 69, pid=34566, name=python3, BAR1 access failure at address 0xffff80001234abcd",
	},
	74: {
		Priority: "KERN_WARNING",
		Message:  "NVRM: Xid (PCI:0000:04:00): 74, pid=1234, name=python3, Channel 0x23, MMU Fault: ENGINE GRAPHICS GPCCLIENT_T1_0 faulted @ 0x7fc123456000. Fault is of type FAULT_PTE ACCESS_TYPE_VIRT_READ",
	},
	79: {
		Priority: "KERN_ERR",
		Message:  "NVRM: Xid (PCI:0000:04:00): 79, GPU has fallen off the bus",
	},
}
