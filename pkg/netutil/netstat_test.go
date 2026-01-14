package netutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNetStatFixtures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		fixture  string
		expected map[string]map[string]uint64
	}{
		{
			name:    "netstat.0 TcpExt sample",
			fixture: "testdata/netstat.0",
			expected: map[string]map[string]uint64{
				"TcpExt": {
					"TCPAbortOnClose":   254856,
					"TCPAbortOnTimeout": 6,
					"TCPRcvCollapsed":   0,
				},
				"IpExt": {
					"OutOctets": 4490491570712,
				},
			},
		},
		{
			name:    "netstat.1 TcpExt sample",
			fixture: "testdata/netstat.1",
			expected: map[string]map[string]uint64{
				"TcpExt": {
					"TCPAbortOnClose":   666665,
					"TCPAbortOnTimeout": 146,
				},
				"IpExt": {
					"InOctets": 17538319886592,
				},
			},
		},
		{
			name:    "netstat.2 TcpExt sample",
			fixture: "testdata/netstat.2",
			expected: map[string]map[string]uint64{
				"TcpExt": {
					"TCPAbortOnClose":   132951,
					"TCPAbortOnTimeout": 21,
				},
				"IpExt": {
					"InNoRoutes": 0,
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, err := os.Open(filepath.Join("testdata", filepath.Base(tc.fixture)))
			require.NoError(t, err)
			defer f.Close()

			stats, err := parseProcNet(f)
			require.NoError(t, err)

			for protocol, fields := range tc.expected {
				protoStats, ok := stats[protocol]
				require.Truef(t, ok, "expected protocol %s", protocol)
				for name, value := range fields {
					got, exists := protoStats[name]
					require.Truef(t, exists, "expected counter %s_%s", protocol, name)
					assert.Equalf(t, value, got, "unexpected value for %s_%s", protocol, name)
				}
			}
		})
	}
}

func TestParseNetStatMismatchedFields(t *testing.T) {
	t.Parallel()

	const malformed = "Tcp: InSegs OutSegs\nTcp: 10"
	_, err := parseProcNet(strings.NewReader(malformed))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field/value mismatch")
}

func TestParseNetStatInvalidProtocolHeader(t *testing.T) {
	t.Parallel()

	const malformed = "Tcp InSegs OutSegs\nTcp: 10 20"
	_, err := parseProcNet(strings.NewReader(malformed))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing trailing colon")
}

func TestReadNetStatCounters(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	write := func(name, content string) string {
		path := filepath.Join(tmpDir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		return path
	}

	// Create netstat file (TcpExt, IpExt, etc.)
	netstatPath := write("netstat", strings.TrimSpace(`
TcpExt: SyncookiesSent TCPSegRetrans
TcpExt: 0 123
`))

	// Create snmp file (Tcp, Udp, etc.)
	snmpPath := write("snmp", strings.TrimSpace(`
Tcp: RtoAlgorithm RtoMin RtoMax MaxConn ActiveOpens PassiveOpens AttemptFails EstabResets CurrEstab InSegs OutSegs RetransSegs InErrs OutRsts InCsumErrors
Tcp: 1 200 120000 -1 100 200 10 20 5 1000 2000 42 3 50 0
Udp: InDatagrams NoPorts InErrors OutDatagrams RcvbufErrors SndbufErrors InCsumErrors IgnoredMulti MemErrors
Udp: 1000 10 2 1100 7 9 0 50 0
`))

	counters, err := readNetStatCounters(netstatPath, snmpPath)
	require.NoError(t, err)

	// From snmp file (Tcp protocol)
	assert.Equal(t, uint64(42), counters.TCPRetransSegments)

	// From netstat file (TcpExt protocol)
	assert.Equal(t, uint64(123), counters.TcpExtSegmentRetransmits)

	// From snmp file (Udp protocol)
	assert.Equal(t, uint64(2), counters.UDPInErrors)
	assert.Equal(t, uint64(7), counters.UDPRcvbufErrors)
	assert.Equal(t, uint64(9), counters.UDPSndbufErrors)
}

func TestReadNetStatCountersWithMissingFiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	netstatPath := filepath.Join(tmpDir, "missing")
	snmpPath := filepath.Join(tmpDir, "missing_snmp")

	counters, err := readNetStatCounters(netstatPath, snmpPath)
	assert.Error(t, err)
	assert.Zero(t, counters)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestReadNetStatCountersWithEmptyNetStatPath(t *testing.T) {
	t.Parallel()

	counters, err := readNetStatCounters("", "/proc/net/snmp")
	assert.Error(t, err)
	assert.Zero(t, counters)
	assert.True(t, errors.Is(err, ErrNoNetStatFile))
}

func TestReadNetStatCountersWithEmptySNMPPath(t *testing.T) {
	t.Parallel()

	counters, err := readNetStatCounters("/proc/net/netstat", "")
	assert.Error(t, err)
	assert.Zero(t, counters)
	assert.True(t, errors.Is(err, ErrNoSNMPFile))
}

func TestReadNetStatCountersWithBothEmptyPaths(t *testing.T) {
	t.Parallel()

	counters, err := readNetStatCounters("", "")
	assert.Error(t, err)
	assert.Zero(t, counters)
	assert.True(t, errors.Is(err, ErrNoNetStatFile)) // Should fail on first check
}

// TestReadSNMPFixtures tests SNMP file parsing with real fixtures
func TestReadSNMPFixtures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		fixture  string
		expected map[string]map[string]uint64
	}{
		{
			name:    "snmp.0 Tcp and Udp counters",
			fixture: "testdata/snmp.0",
			expected: map[string]map[string]uint64{
				"Tcp": {
					"RetransSegs": 73095,
					"InSegs":      380609877,
					"OutSegs":     419685271,
					"InErrs":      128,
				},
				"Udp": {
					"InErrors":     6979,
					"RcvbufErrors": 6979,
					"SndbufErrors": 0,
					"InDatagrams":  402301656,
					"OutDatagrams": 1094611811,
				},
			},
		},
		{
			name:    "snmp.1 Tcp and Udp counters",
			fixture: "testdata/snmp.1",
			expected: map[string]map[string]uint64{
				"Tcp": {
					"RetransSegs": 218765,
					"InSegs":      551593608,
					"OutSegs":     1042962688,
					"InErrs":      99,
				},
				"Udp": {
					"InErrors":     10010,
					"RcvbufErrors": 10010,
					"SndbufErrors": 0,
					"InDatagrams":  504194467,
					"OutDatagrams": 1115130484,
				},
			},
		},
		{
			name:    "snmp.2 Tcp and Udp counters",
			fixture: "testdata/snmp.2",
			expected: map[string]map[string]uint64{
				"Tcp": {
					"RetransSegs": 797471,
					"InSegs":      735295931,
					"OutSegs":     1015429306,
					"InErrs":      472,
				},
				"Udp": {
					"InErrors":     8652,
					"RcvbufErrors": 8652,
					"SndbufErrors": 0,
					"InDatagrams":  828805710,
					"OutDatagrams": 2267755130,
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a temporary dummy netstat file (empty content is fine for this test)
			tmpDir := t.TempDir()
			dummyNetstat := filepath.Join(tmpDir, "netstat")
			require.NoError(t, os.WriteFile(dummyNetstat, []byte("TcpExt: Dummy\nTcpExt: 0\n"), 0o644))

			// Test parsing SNMP file with dummy netstat file
			counters, err := readNetStatCounters(dummyNetstat, tc.fixture)
			require.NoError(t, err)

			// Verify TCP counters
			if tcpExpected, ok := tc.expected["Tcp"]; ok {
				if retrans, ok := tcpExpected["RetransSegs"]; ok {
					assert.Equal(t, retrans, counters.TCPRetransSegments)
				}
			}

			// Verify UDP counters
			if udpExpected, ok := tc.expected["Udp"]; ok {
				if inErr, ok := udpExpected["InErrors"]; ok {
					assert.Equal(t, inErr, counters.UDPInErrors)
				}
				if rcvbuf, ok := udpExpected["RcvbufErrors"]; ok {
					assert.Equal(t, rcvbuf, counters.UDPRcvbufErrors)
				}
				if sndbuf, ok := udpExpected["SndbufErrors"]; ok {
					assert.Equal(t, sndbuf, counters.UDPSndbufErrors)
				}
			}
		})
	}
}

func TestReadNetStatCountersWithMalformedSNMP(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create dummy netstat file
	dummyNetstat := filepath.Join(tmpDir, "netstat")
	require.NoError(t, os.WriteFile(dummyNetstat, []byte("TcpExt: Dummy\nTcpExt: 0\n"), 0o644))

	// Create malformed SNMP file (mismatched field/value counts)
	malformedPath := filepath.Join(tmpDir, "malformed")
	const malformed = "Tcp: RetransSegs InSegs\nTcp: 100"
	require.NoError(t, os.WriteFile(malformedPath, []byte(malformed), 0o644))

	counters, err := readNetStatCounters(dummyNetstat, malformedPath)
	assert.Error(t, err)
	assert.Zero(t, counters)
	assert.Contains(t, err.Error(), "field/value mismatch")
}

func TestReadNetStatCountersVerifySNMPFormat(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create dummy netstat file
	dummyNetstat := filepath.Join(tmpDir, "netstat")
	require.NoError(t, os.WriteFile(dummyNetstat, []byte("TcpExt: Dummy\nTcpExt: 0\n"), 0o644))

	// Create a valid SNMP file with test data
	snmpPath := filepath.Join(tmpDir, "snmp")
	snmpData := strings.TrimSpace(`
Tcp: RtoAlgorithm RtoMin RtoMax MaxConn ActiveOpens PassiveOpens AttemptFails EstabResets CurrEstab InSegs OutSegs RetransSegs InErrs OutRsts InCsumErrors
Tcp: 1 200 120000 -1 100 200 10 20 5 1000 2000 42 3 50 0
Udp: InDatagrams NoPorts InErrors OutDatagrams RcvbufErrors SndbufErrors InCsumErrors IgnoredMulti MemErrors
Udp: 5000 10 15 6000 20 25 0 100 0
`)
	require.NoError(t, os.WriteFile(snmpPath, []byte(snmpData), 0o644))

	counters, err := readNetStatCounters(dummyNetstat, snmpPath)
	require.NoError(t, err)

	// Verify Tcp counters
	assert.Equal(t, uint64(42), counters.TCPRetransSegments)

	// Verify Udp counters
	assert.Equal(t, uint64(15), counters.UDPInErrors)
	assert.Equal(t, uint64(25), counters.UDPSndbufErrors)
	assert.Equal(t, uint64(20), counters.UDPRcvbufErrors)
}
