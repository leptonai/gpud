package v1

import (
	"time"

	"github.com/dustin/go-humanize"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KernelMessage is the kernel record included in GPUd component results.
// BootID is used for delivery identity and is omitted from the existing JSON format.
type KernelMessage struct {
	Timestamp      metav1.Time `json:"timestamp"`
	Priority       int         `json:"priority"`
	SequenceNumber int         `json:"sequence_number"`
	Message        string      `json:"message"`
	BootID         string      `json:"-"`
}

func (m KernelMessage) DescribeTimestamp(since time.Time) string {
	return humanize.RelTime(m.Timestamp.Time, since, "ago", "from now")
}
