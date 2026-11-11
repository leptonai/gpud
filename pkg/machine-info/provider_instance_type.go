package machineinfo

import (
	"context"
	"strings"
	"sync"
	"time"

	pkgprovidersall "github.com/leptonai/gpud/pkg/providers/all"
)

// InstanceTypeCollector keeps optional IMDS requests off the gossip request path.
type InstanceTypeCollector struct {
	mu    sync.RWMutex
	value string
}

// NewInstanceTypeCollector refreshes metadata until the session is closed.
func NewInstanceTypeCollector(ctx context.Context) *InstanceTypeCollector {
	c := &InstanceTypeCollector{}
	go c.run(ctx, 5*time.Minute, pkgprovidersall.DetectInstanceType)
	return c
}

// Latest returns the last successful reading; missing metadata never clears it.
func (c *InstanceTypeCollector) Latest() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value
}

func (c *InstanceTypeCollector) run(ctx context.Context, interval time.Duration, detect func(context.Context) string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		cctx, cancel := context.WithTimeout(ctx, time.Minute)
		value := strings.TrimSpace(detect(cctx))
		cancel()
		if value != "" && !strings.EqualFold(value, "unknown") {
			c.mu.Lock()
			c.value = value
			c.mu.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
