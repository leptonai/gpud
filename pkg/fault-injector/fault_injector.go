// Package faultinjector provides a way to inject failures into the system.
package faultinjector

import (
	"errors"

	xidquery "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
)

// Injector defines the interface for injecting failures into the system.
type Injector interface {
	KmsgWriter() KmsgWriter
}

func NewInjector(kmsgWriter KmsgWriter) Injector {
	return &injector{
		kmsgWriter: kmsgWriter,
	}
}

type injector struct {
	kmsgWriter KmsgWriter
}

func (i *injector) KmsgWriter() KmsgWriter {
	return i.kmsgWriter
}

// Request is the request body for the inject-fault endpoint.
type Request struct {
	// XID is the XID to inject.
	XID *XIDToInject `json:"xid,omitempty"`

	// KernelMessage is the kernel message to inject.
	KernelMessage *KernelMessage `json:"kernel_message,omitempty"`
}

type XIDToInject struct {
	ID int `json:"id"`
}

var ErrNoFaultFound = errors.New("no fault injection entry found")

func (r *Request) Validate() error {
	switch {
	case r.XID != nil:
		if r.XID.ID == 0 {
			return ErrNoFaultFound
		}

		msg := xidquery.GetMessageToInject(r.XID.ID)
		r.KernelMessage = &KernelMessage{
			Priority: ConvertKernelMessagePriority(msg.Priority),
			Message:  msg.Message,
		}
		r.XID = nil

		// fall through to a subsequent case to call Validate()
		fallthrough

	case r.KernelMessage != nil:
		return r.KernelMessage.Validate()

	default:
		return ErrNoFaultFound
	}
}
