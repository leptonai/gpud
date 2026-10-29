package v1

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKernelMessagePreservesJSON(t *testing.T) {
	at := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	record := KernelMessage{Timestamp: metav1.NewTime(at), Priority: 6, SequenceNumber: 12, Message: "body", BootID: "internal-identity"}
	data, err := json.Marshal(record)
	require.NoError(t, err)
	assert.JSONEq(t, `{"timestamp":"2026-10-29T00:00:00Z","priority":6,"sequence_number":12,"message":"body"}`, string(data))
	assert.Equal(t, "now", record.DescribeTimestamp(at))
}
