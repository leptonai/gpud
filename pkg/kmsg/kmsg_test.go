package kmsg

import (
	"bufio"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMessage(t *testing.T) {
	bootTime := time.Unix(0xb100, 0x5ea1).Round(time.Microsecond)

	msg, err := parseMessage(bootTime, "6,2565,102258085667,-;docker0: port 2(vethc1bb733) entered blocking state")
	require.NoError(t, err)

	assert.Equal(t, msg.Message, "docker0: port 2(vethc1bb733) entered blocking state")
	assert.Equal(t, msg.Priority, 6)
	assert.Equal(t, msg.SequenceNumber, 2565)
	assert.Equal(t, msg.Timestamp, bootTime.Add(102258085667*time.Microsecond))
}

func TestReadAll(t *testing.T) {
	bootTime := time.Unix(0xb100, 0x5ea1).Round(time.Microsecond)

	f, err := os.Open("testdata/kmsg.1.log")
	require.NoError(t, err)
	defer f.Close()

	buf := bufio.NewScanner(f)
	for buf.Scan() {
		line := buf.Text()
		if len(line) == 0 {
			continue
		}
		msg, err := parseMessage(bootTime, line)
		require.NotNil(t, msg)
		require.NoError(t, err)
	}
}
