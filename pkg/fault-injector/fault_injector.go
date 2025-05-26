// Package faultinjector provides a way to inject failures into the system.
package faultinjector

import (
	"errors"

	componentsnvidiaxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	pkgkmsgwriter "github.com/leptonai/gpud/pkg/kmsg/writer"
)

// Injector defines the interface for injecting failures into the system.
type Injector interface {
	pkgkmsgwriter.GPUdKmsgWriterModule
}

// Request is the request body for the inject-fault endpoint.
type Request struct {
	// XidToInject is the XID to inject.
	Xid *XidToInject `json:"xid,omitempty"`

	// KernelMessage is the kernel message to inject.
	KernelMessage *pkgkmsgwriter.KernelMessage `json:"kernel_message,omitempty"`
}

type XidToInject struct {
	ID int `json:"id"`
}

var ErrNoFaultFound = errors.New("no fault injection entry found")

func (r *Request) Validate() error {
	switch {
	case r.Xid != nil:
		if r.Xid.ID == 0 {
			return ErrNoFaultFound
		}

		msg := componentsnvidiaxid.GetMessageToInject(r.Xid.ID)
		r.KernelMessage = &pkgkmsgwriter.KernelMessage{
			Priority: pkgkmsgwriter.ConvertKernelMessagePriority(msg.Priority),
			Message:  msg.Message,
		}
		r.Xid = nil

		// fall through to a subsequent case to call Validate()
		fallthrough

	case r.KernelMessage != nil:
		return r.KernelMessage.Validate()

	default:
		return ErrNoFaultFound
	}
}
