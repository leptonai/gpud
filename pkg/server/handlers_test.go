package server

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/metrics"
)

func TestNewGlobalHandler(t *testing.T) {
	var metricStore metrics.Store
	ghler := newGlobalHandler(nil, components.NewRegistry(nil), metricStore)
	assert.NotNil(t, ghler)
}

func TestGetReqTime(t *testing.T) {
	g := &globalHandler{}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		startTimeQuery string
		endTimeQuery   string
		expectError    bool
	}{
		{
			name:           "no query params",
			startTimeQuery: "",
			endTimeQuery:   "",
			expectError:    false,
		},
		{
			name:           "valid start and end times",
			startTimeQuery: "1609459200", // 2021-01-01 00:00:00
			endTimeQuery:   "1609545600", // 2021-01-02 00:00:00
			expectError:    false,
		},
		{
			name:           "invalid start time",
			startTimeQuery: "invalid",
			endTimeQuery:   "1609545600",
			expectError:    true,
		},
		{
			name:           "invalid end time",
			startTimeQuery: "1609459200",
			endTimeQuery:   "invalid",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest("GET", "/?startTime="+tt.startTimeQuery+"&endTime="+tt.endTimeQuery, nil)
			c.Request = req

			startTime, endTime, err := g.getReqTime(c)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.startTimeQuery != "" {
					expectedStartTime := time.Unix(1609459200, 0)
					assert.Equal(t, expectedStartTime, startTime)
				}
				if tt.endTimeQuery != "" {
					expectedEndTime := time.Unix(1609545600, 0)
					assert.Equal(t, expectedEndTime, endTime)
				}
			}
		})
	}
}
