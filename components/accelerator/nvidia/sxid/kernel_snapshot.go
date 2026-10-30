package sxid

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	kmsgsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/kmsg"

	apiv1 "github.com/leptonai/gpud/api/v1"
	componentkmsg "github.com/leptonai/gpud/components/kmsg"
	"github.com/leptonai/gpud/pkg/eventstore"
)

func readKernelMessages(ctx context.Context) ([]apiv1.KernelMessage, error) {
	if path := strings.TrimSpace(os.Getenv("KMSG_FILE_PATH")); path != "" && path != "/dev/kmsg" {
		return nil, fmt.Errorf("health-validation reads /dev/kmsg; KMSG_FILE_PATH=%q is unsupported", path)
	}
	sensor, err := kmsgsensor.New(nil, map[string]string{"gpud.kernel_record": `(?s).*`})
	if err != nil {
		return nil, err
	}
	batch, err := componentkmsg.Measure(ctx, sensor)
	if err != nil {
		return nil, err
	}
	records, err := componentkmsg.Messages(batch)
	dedup := eventstore.NewDeduplicator(time.Minute)
	messages := make([]apiv1.KernelMessage, 0, len(records))
	for _, record := range records {
		if dedup.Accept(record.Timestamp.Time, record.Message) {
			messages = append(messages, record)
		}
	}
	return messages, err
}
