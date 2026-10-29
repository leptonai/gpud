package xid

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	kmsgsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/kmsg"
	kmsgsignals "github.com/dsx-ai-factory/health-validation/collect/signals/kmsg"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
)

func readKernelMessages(ctx context.Context) ([]kernelRecord, error) {
	if path := strings.TrimSpace(os.Getenv("KMSG_FILE_PATH")); path != "" && path != "/dev/kmsg" {
		return nil, fmt.Errorf("health-validation reads /dev/kmsg; KMSG_FILE_PATH=%q is unsupported", path)
	}
	sensor, err := kmsgsensor.New(nil, map[string]string{"gpud.kernel_record": `(?s).*`})
	if err != nil {
		return nil, err
	}
	batch, err := components.MeasureSensor(ctx, sensor, kmsgsignals.Message.ID(), kmsgsignals.Priority.ID())
	if err != nil {
		return nil, err
	}
	records, err := kernelRecords(batch)
	dedup := eventstore.NewDeduplicator(time.Minute)
	messages := make([]kernelRecord, 0, len(records))
	for _, record := range records {
		if dedup.Accept(record.Timestamp.Time, record.Message) {
			messages = append(messages, record)
		}
	}
	return messages, err
}
