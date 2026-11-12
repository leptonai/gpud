package machineinfo

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInstanceTypeCollectorRefreshAndCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		collector := &InstanceTypeCollector{}
		calls := 0
		done := make(chan struct{})
		go func() {
			defer close(done)
			collector.run(ctx, 5*time.Minute, func(ctx context.Context) string {
				calls++
				switch calls {
				case 1:
					<-ctx.Done()
					return ""
				case 2:
					return " Standard_ND96isr_H100_v5 "
				case 3:
					return "unknown"
				default:
					return "Standard_ND96isr_H200_v5"
				}
			})
		}()
		synctest.Wait()
		assert.Empty(t, collector.Latest(), "reads must not wait for IMDS")
		time.Sleep(5 * time.Minute)
		synctest.Wait()
		assert.Equal(t, "Standard_ND96isr_H100_v5", collector.Latest())
		time.Sleep(5 * time.Minute)
		synctest.Wait()
		assert.Equal(t, "Standard_ND96isr_H100_v5", collector.Latest(), "failed refresh retains the last reading")
		time.Sleep(5 * time.Minute)
		synctest.Wait()
		assert.Equal(t, "Standard_ND96isr_H200_v5", collector.Latest())
		cancel()
		<-done
		assert.Equal(t, 4, calls)
	})
}
