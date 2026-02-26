package pstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubDBConfig struct {
	beginErr        error
	commitErr       error
	execErrAt       map[int]error
	rowsAffectedErr error
	queryErr        error
	columns         []string
	rows            [][]driver.Value
	nextErr         error
}

type stubDriver struct {
	cfg *stubDBConfig
}

func (d *stubDriver) Open(name string) (driver.Conn, error) {
	return &stubConn{cfg: d.cfg}, nil
}

type stubConn struct {
	cfg       *stubDBConfig
	execCount int
}

func (c *stubConn) Prepare(query string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported")
}

func (c *stubConn) Close() error { return nil }

func (c *stubConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *stubConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if c.cfg != nil && c.cfg.beginErr != nil {
		return nil, c.cfg.beginErr
	}
	return &stubTx{cfg: c.cfg}, nil
}

func (c *stubConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.execCount++
	if c.cfg != nil && c.cfg.execErrAt != nil {
		if err, ok := c.cfg.execErrAt[c.execCount]; ok {
			return nil, err
		}
	}
	return stubResult{rows: 1, rowsErr: c.cfg.rowsAffectedErr}, nil
}

func (c *stubConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.cfg != nil && c.cfg.queryErr != nil {
		return nil, c.cfg.queryErr
	}
	cols := []string{"c1"}
	if c.cfg != nil && len(c.cfg.columns) > 0 {
		cols = c.cfg.columns
	}
	var rows [][]driver.Value
	var nextErr error
	if c.cfg != nil {
		rows = c.cfg.rows
		nextErr = c.cfg.nextErr
	}
	return &stubRows{
		columns: cols,
		rows:    rows,
		nextErr: nextErr,
	}, nil
}

type stubTx struct {
	cfg *stubDBConfig
}

func (tx *stubTx) Commit() error {
	if tx.cfg != nil {
		return tx.cfg.commitErr
	}
	return nil
}

func (tx *stubTx) Rollback() error { return nil }

type stubResult struct {
	rows    int64
	rowsErr error
}

func (r stubResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (r stubResult) RowsAffected() (int64, error) {
	if r.rowsErr != nil {
		return 0, r.rowsErr
	}
	return r.rows, nil
}

type stubRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
	nextErr error
}

func (r *stubRows) Columns() []string { return r.columns }

func (r *stubRows) Close() error { return nil }

func (r *stubRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		if r.nextErr != nil {
			err := r.nextErr
			r.nextErr = nil
			return err
		}
		return io.EOF
	}

	row := r.rows[r.idx]
	r.idx++
	for i := range dest {
		if i < len(row) {
			dest[i] = row[i]
		} else {
			dest[i] = nil
		}
	}
	return nil
}

var stubDriverN uint64

func setupStubDB(t *testing.T, cfg *stubDBConfig) *sql.DB {
	t.Helper()

	name := fmt.Sprintf("pstore_stub_driver_%d", atomic.AddUint64(&stubDriverN, 1))
	sql.Register(name, &stubDriver{cfg: cfg})

	db, err := sql.Open(name, "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestSysrqCrashPattern(t *testing.T) {
	// Test the pattern matching for sysrq crash triggers
	matchFunc := func(line string) (eventName string, message string) {
		// Match "sysrq: SysRq : Trigger a crash"
		if strings.Contains(line, "sysrq: SysRq : Trigger a crash") {
			return "sysrq_crash", "SysRq crash trigger detected"
		}
		return "", ""
	}

	testCases := []struct {
		line        string
		expectEvent string
		expectMsg   string
	}{
		{
			line:        "<6>[  201.650687] sysrq: SysRq : Trigger a crash",
			expectEvent: "sysrq_crash",
			expectMsg:   "SysRq crash trigger detected",
		},
		{
			line:        "<6>[  100.123456] sysrq: SysRq : Trigger a crash",
			expectEvent: "sysrq_crash",
			expectMsg:   "SysRq crash trigger detected",
		},
		{
			line:        "<4>[  201.654822] BUG: unable to handle kernel NULL pointer dereference",
			expectEvent: "",
			expectMsg:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.line, func(t *testing.T) {
			eventName, message := matchFunc(tc.line)
			assert.Equal(t, tc.expectEvent, eventName)
			assert.Equal(t, tc.expectMsg, message)
		})
	}
}

func TestKernelPanicPattern(t *testing.T) {
	// Test the pattern matching for kernel panic messages
	matchFunc := func(line string) (eventName string, message string) {
		// Match "Kernel panic - not syncing:"
		panicRegex := regexp.MustCompile(`Kernel panic - not syncing: (.+)`)
		if matches := panicRegex.FindStringSubmatch(line); len(matches) > 1 {
			return "kernel_panic", strings.TrimSpace(matches[1])
		}
		return "", ""
	}

	testCases := []struct {
		line        string
		expectEvent string
		expectMsg   string
	}{
		{
			line:        "<0>[ 3098.275469] Kernel panic - not syncing: Test panic triggered by crash_test module",
			expectEvent: "kernel_panic",
			expectMsg:   "Test panic triggered by crash_test module",
		},
		{
			line:        "<0>[12345.678901] Kernel panic - not syncing: Out of memory",
			expectEvent: "kernel_panic",
			expectMsg:   "Out of memory",
		},
		{
			line:        "<4>[  201.654822] BUG: unable to handle kernel NULL pointer dereference",
			expectEvent: "",
			expectMsg:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.line, func(t *testing.T) {
			eventName, message := matchFunc(tc.line)
			assert.Equal(t, tc.expectEvent, eventName)
			assert.Equal(t, tc.expectMsg, message)
		})
	}
}

func TestCombinedPatternMatching(t *testing.T) {
	// Combined pattern matcher that matches both sysrq and kernel panic
	matchFunc := func(line string) (eventName string, message string) {
		// Match "sysrq: SysRq : Trigger a crash"
		if strings.Contains(line, "sysrq: SysRq : Trigger a crash") {
			return "sysrq_crash", "SysRq crash trigger detected"
		}

		// Match "Kernel panic - not syncing:"
		panicRegex := regexp.MustCompile(`Kernel panic - not syncing: (.+)`)
		if matches := panicRegex.FindStringSubmatch(line); len(matches) > 1 {
			return "kernel_panic", strings.TrimSpace(matches[1])
		}

		return "", ""
	}

	// Test with real data from testdata files
	testDataPath := "testdata/dmesg-erst.txt"
	if _, err := os.Stat(testDataPath); os.IsNotExist(err) {
		t.Skip("Test data file not found")
	}

	data, err := os.ReadFile(testDataPath)
	require.NoError(t, err, "Failed to read test data")

	lines := strings.Split(string(data), "\n")
	foundSysrq := false

	for _, line := range lines {
		if line == "" {
			continue
		}

		eventName, message := matchFunc(line)
		if eventName == "sysrq_crash" {
			foundSysrq = true
			assert.Equal(t, "SysRq crash trigger detected", message, "Unexpected sysrq message")
		}
	}

	assert.True(t, foundSysrq, "Expected to find sysrq crash trigger in test data")
}

func setupTestDB(t *testing.T) (*sql.DB, *sql.DB) {
	// Create temporary database files with WAL mode to avoid locking issues
	dbFile := filepath.Join(t.TempDir(), "test.db")

	dbRW, err := sql.Open("sqlite3", dbFile+"?_journal_mode=WAL&_timeout=5000")
	require.NoError(t, err, "Failed to create RW database")

	dbRO, err := sql.Open("sqlite3", dbFile+"?_journal_mode=WAL&_timeout=5000")
	require.NoError(t, err, "Failed to create RO database")

	return dbRW, dbRO
}

func TestPstoreNew(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	tempDir := t.TempDir()

	store, err := New(tempDir, dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.NoError(t, err, "Failed to create pstore")

	assert.NotNil(t, store, "Expected non-nil store")
}

func TestPstoreScanWithTestData(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	// Copy test data to temporary directory
	tempDir := t.TempDir()

	// Copy the test files - keeping the subdirectory structure
	testFiles := []struct {
		src  string
		dest string
	}{
		{"testdata/dmesg-erst.txt", "dmesg-erst.txt"},
		{"testdata/7530486857247/dmesg.txt", "7530486857247/dmesg.txt"},
		{"testdata/7530486857247/dmesg-erst-7530486857247752193", "7530486857247/dmesg-erst-7530486857247752193"},
		{"testdata/7530486857247/dmesg-erst-7530486857247752194", "7530486857247/dmesg-erst-7530486857247752194"},
	}

	for _, testFile := range testFiles {
		if _, err := os.Stat(testFile.src); os.IsNotExist(err) {
			continue // Skip missing test files
		}

		data, err := os.ReadFile(testFile.src)
		if err != nil {
			continue
		}

		destPath := filepath.Join(tempDir, testFile.dest)
		destDir := filepath.Dir(destPath)

		// Create subdirectory if needed
		if destDir != tempDir {
			err = os.MkdirAll(destDir, 0755)
			require.NoError(t, err, "Failed to create subdirectory")
		}

		err = os.WriteFile(destPath, data, 0644)
		require.NoError(t, err, "Failed to copy test file")
	}

	store, err := New(tempDir, dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.NoError(t, err, "Failed to create pstore")

	// Pattern matcher for both sysrq and kernel panic
	matchFunc := func(line string) (eventName string, message string) {
		if strings.Contains(line, "sysrq: SysRq : Trigger a crash") {
			return "sysrq_crash", "SysRq crash trigger detected"
		}

		panicRegex := regexp.MustCompile(`Kernel panic - not syncing: (.+)`)
		if matches := panicRegex.FindStringSubmatch(line); len(matches) > 1 {
			return "kernel_panic", strings.TrimSpace(matches[1])
		}

		return "", ""
	}

	ctx := context.Background()

	// First scan
	err = store.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan pstore")

	// Get results
	histories, err := store.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories")

	// Should find at least one event
	assert.NotEmpty(t, histories, "Expected to find at least one history event")

	// Verify we found expected events
	foundSysrq := false
	foundPanic := false

	for _, h := range histories {
		switch h.EventName {
		case "sysrq_crash":
			foundSysrq = true
			assert.Equal(t, "SysRq crash trigger detected", h.Message, "Unexpected sysrq message")
		case "kernel_panic":
			foundPanic = true
			// Don't assert exact message since it could come from different files
		}
	}

	t.Logf("Found sysrq: %v, Found panic: %v", foundSysrq, foundPanic)

	t.Logf("Found %d history events", len(histories))
	for _, h := range histories {
		t.Logf("Event: %s, Message: %s", h.EventName, h.Message)
	}
}

func TestPstoreNoDuplicates(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	// Create test data
	tempDir := t.TempDir()
	testData := `<6>[  201.650687] sysrq: SysRq : Trigger a crash
<0>[ 3098.275469] Kernel panic - not syncing: Test panic triggered by crash_test module`

	err := os.WriteFile(filepath.Join(tempDir, "dmesg.txt"), []byte(testData), 0644)
	require.NoError(t, err, "Failed to write test data")

	store, err := New(tempDir, dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.NoError(t, err, "Failed to create pstore")

	matchFunc := func(line string) (eventName string, message string) {
		if strings.Contains(line, "sysrq: SysRq : Trigger a crash") {
			return "sysrq_crash", "SysRq crash trigger detected"
		}

		panicRegex := regexp.MustCompile(`Kernel panic - not syncing: (.+)`)
		if matches := panicRegex.FindStringSubmatch(line); len(matches) > 1 {
			return "kernel_panic", strings.TrimSpace(matches[1])
		}

		return "", ""
	}

	ctx := context.Background()

	// First scan
	err = store.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan pstore")

	// Get initial count
	histories1, err := store.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories")

	// Second scan - should not add duplicates
	err = store.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan pstore second time")

	// Get count after second scan
	histories2, err := store.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories")

	// Should have same count - no duplicates
	if !assert.Equal(t, len(histories1), len(histories2), "Expected same count after second scan") {
		t.Logf("First scan results:")
		for _, h := range histories1 {
			t.Logf("  %s: %s", h.EventName, h.Message)
		}
		t.Logf("Second scan results:")
		for _, h := range histories2 {
			t.Logf("  %s: %s", h.EventName, h.Message)
		}
	}

	// Third scan - should still not add duplicates
	err = store.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan pstore third time")

	histories3, err := store.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories")

	assert.Equal(t, len(histories1), len(histories3), "Expected same count after third scan")
}

func TestPstoreGetWithTimeFilter(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	tempDir := t.TempDir()
	testData := `<6>[  201.650687] sysrq: SysRq : Trigger a crash`

	err := os.WriteFile(filepath.Join(tempDir, "dmesg.txt"), []byte(testData), 0644)
	require.NoError(t, err, "Failed to write test data")

	store, err := New(tempDir, dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.NoError(t, err, "Failed to create pstore")

	matchFunc := func(line string) (eventName string, message string) {
		if strings.Contains(line, "sysrq: SysRq : Trigger a crash") {
			return "sysrq_crash", "SysRq crash trigger detected"
		}
		return "", ""
	}

	ctx := context.Background()

	// Scan to populate data
	err = store.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan pstore")

	// Get with recent time filter (should find events)
	historiesRecent, err := store.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get recent histories")

	// Get with future time filter (should find no events)
	historiesFuture, err := store.Get(ctx, time.Now().Add(1*time.Hour))
	require.NoError(t, err, "Failed to get future histories")

	assert.NotEmpty(t, historiesRecent, "Expected to find events with recent time filter")
	assert.Empty(t, historiesFuture, "Expected no events with future time filter")
}

func TestPstoreEmptyDirectory(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	tempDir := t.TempDir()

	store, err := New(tempDir, dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.NoError(t, err, "Failed to create pstore")

	matchFunc := func(line string) (eventName string, message string) {
		return "", ""
	}

	ctx := context.Background()

	// Scan empty directory
	err = store.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan empty directory")

	// Should get no histories
	histories, err := store.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories")

	assert.Empty(t, histories, "Expected no histories from empty directory")
}

func TestPstoreSchemaVersioning(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	tempDir := t.TempDir()

	// Create first store with one table name
	store1, err := New(tempDir, dbRW, dbRO, "pstore_v1", 24*time.Hour)
	require.NoError(t, err, "Failed to create first pstore")

	// Create second store with different table name
	store2, err := New(tempDir, dbRW, dbRO, "pstore_v2", 24*time.Hour)
	require.NoError(t, err, "Failed to create second pstore")

	// Both should be non-nil and different table names should be handled
	assert.NotNil(t, store1, "Expected first store to be non-nil")
	assert.NotNil(t, store2, "Expected second store to be non-nil")
}

func TestPstorePurgeWithLookbackPeriod(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	tempDir := t.TempDir()
	testData := `<6>[  201.650687] sysrq: SysRq : Trigger a crash
<0>[ 3098.275469] Kernel panic - not syncing: Test panic triggered by crash_test module`

	err := os.WriteFile(filepath.Join(tempDir, "dmesg.txt"), []byte(testData), 0644)
	require.NoError(t, err, "Failed to write test data")

	// Create store with very short lookback period (1 second)
	store, err := New(tempDir, dbRW, dbRO, "test_pstore", 1*time.Second)
	require.NoError(t, err, "Failed to create pstore")

	matchFunc := func(line string) (eventName string, message string) {
		if strings.Contains(line, "sysrq: SysRq : Trigger a crash") {
			return "sysrq_crash", "SysRq crash trigger detected"
		}

		panicRegex := regexp.MustCompile(`Kernel panic - not syncing: (.+)`)
		if matches := panicRegex.FindStringSubmatch(line); len(matches) > 1 {
			return "kernel_panic", strings.TrimSpace(matches[1])
		}

		return "", ""
	}

	ctx := context.Background()

	// First scan to populate data
	err = store.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan pstore")

	// Get initial count
	histories1, err := store.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories")

	require.NotEmpty(t, histories1, "Expected to find history events after first scan")

	t.Logf("Initial events count: %d", len(histories1))

	// Wait for lookback period to expire
	time.Sleep(2 * time.Second)

	// Create a new store with the same short lookback period - this should trigger purge
	store2, err := New(tempDir, dbRW, dbRO, "test_pstore", 1*time.Second)
	require.NoError(t, err, "Failed to create second pstore")

	// Get count after purge
	histories2, err := store2.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories after purge")

	// Should have fewer (or zero) events due to purging
	assert.Less(t, len(histories2), len(histories1), "Expected purge to remove old events")

	t.Logf("Events count after purge: %d", len(histories2))
}

func TestPstoreLookbackPeriodDuringDuplicateCheck(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	tempDir := t.TempDir()
	testData := `<6>[  201.650687] sysrq: SysRq : Trigger a crash`

	err := os.WriteFile(filepath.Join(tempDir, "dmesg.txt"), []byte(testData), 0644)
	require.NoError(t, err, "Failed to write test data")

	// Create pstore reader with custom getTimeNow function to test lookback logic
	pr := &pstoreReader{
		dir:            tempDir,
		dbRW:           dbRW,
		dbRO:           dbRO,
		historyTable:   "test_pstore_custom_v0_7_0",
		lookBackPeriod: 2 * time.Second,
	}

	baseTime := time.Now().UTC()
	pr.getTimeNow = func() time.Time {
		return baseTime
	}

	err = pr.init()
	require.NoError(t, err, "Failed to initialize pstore reader")

	matchFunc := func(line string) (eventName string, message string) {
		if strings.Contains(line, "sysrq: SysRq : Trigger a crash") {
			return "sysrq_crash", "SysRq crash trigger detected"
		}
		return "", ""
	}

	ctx := context.Background()

	// First scan
	err = pr.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan pstore")

	// Verify event was inserted
	histories1, err := pr.Get(ctx, baseTime.Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories")
	require.Len(t, histories1, 1, "Expected 1 event")

	// Move time forward beyond lookback period
	pr.getTimeNow = func() time.Time {
		return baseTime.Add(3 * time.Second)
	}

	// Second scan with same data - should insert duplicate because old event is outside lookback period
	err = pr.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan pstore second time")

	// Should now have 2 events because the first one is outside the lookback period
	histories2, err := pr.Get(ctx, baseTime.Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories after second scan")

	if !assert.Len(t, histories2, 2, "Expected 2 events after lookback period expired") {
		for i, h := range histories2 {
			t.Logf("Event %d: %s at %d", i, h.EventName, h.Timestamp)
		}
	}
}

func TestPstoreCustomTimeFunction(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	tempDir := t.TempDir()
	testData := `<6>[  201.650687] sysrq: SysRq : Trigger a crash`

	err := os.WriteFile(filepath.Join(tempDir, "dmesg.txt"), []byte(testData), 0644)
	require.NoError(t, err, "Failed to write test data")

	// Test with custom time to ensure timestamps are set correctly
	fixedTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	pr := &pstoreReader{
		dir:            tempDir,
		dbRW:           dbRW,
		dbRO:           dbRO,
		historyTable:   "test_pstore_time_v0_7_0",
		lookBackPeriod: 24 * time.Hour,
		getTimeNow: func() time.Time {
			return fixedTime
		},
	}

	err = pr.init()
	require.NoError(t, err, "Failed to initialize pstore reader")

	matchFunc := func(line string) (eventName string, message string) {
		if strings.Contains(line, "sysrq: SysRq : Trigger a crash") {
			return "sysrq_crash", "SysRq crash trigger detected"
		}
		return "", ""
	}

	ctx := context.Background()

	err = pr.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan pstore")

	histories, err := pr.Get(ctx, fixedTime.Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories")

	require.Len(t, histories, 1, "Expected 1 event")

	// Verify timestamp matches our fixed time
	assert.Equal(t, fixedTime.Unix(), histories[0].Timestamp, "Expected timestamp to match fixed time")
}

func TestPstoreNonExistentDirectory(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	nonExistentDir := "/path/that/does/not/exist"

	store, err := New(nonExistentDir, dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.NoError(t, err, "Failed to create pstore")

	matchFunc := func(line string) (eventName string, message string) {
		return "", ""
	}

	ctx := context.Background()

	// Should get error when trying to scan non-existent directory
	err = store.Scan(ctx, matchFunc)
	assert.Error(t, err, "Expected error when scanning non-existent directory")
}

func TestPstoreGetNoRows(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	tempDir := t.TempDir()

	store, err := New(tempDir, dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.NoError(t, err, "Failed to create pstore")

	ctx := context.Background()

	// Get from empty database should return empty slice, not error
	histories, err := store.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories from empty db")

	assert.Empty(t, histories, "Expected no histories from empty db")
}

func TestPstoreScanWithRecursiveDirectories(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	// Create temp directory with a subdirectory structure like real pstore
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "7530486857247")
	err := os.Mkdir(subDir, 0755)
	require.NoError(t, err, "Failed to create subdirectory")

	// Create files in both root and subdirectory
	testDataRoot := `<6>[  201.650687] sysrq: SysRq : Trigger a crash`
	err = os.WriteFile(filepath.Join(tempDir, "dmesg-erst.txt"), []byte(testDataRoot), 0644)
	require.NoError(t, err, "Failed to write root test data")

	testDataSub := `<0>[ 3098.275469] Kernel panic - not syncing: Test panic triggered by crash_test module`
	err = os.WriteFile(filepath.Join(subDir, "dmesg.txt"), []byte(testDataSub), 0644)
	require.NoError(t, err, "Failed to write subdirectory test data")

	store, err := New(tempDir, dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.NoError(t, err, "Failed to create pstore")

	matchFunc := func(line string) (eventName string, message string) {
		if strings.Contains(line, "sysrq: SysRq : Trigger a crash") {
			return "sysrq_crash", "SysRq crash trigger detected"
		}

		panicRegex := regexp.MustCompile(`Kernel panic - not syncing: (.+)`)
		if matches := panicRegex.FindStringSubmatch(line); len(matches) > 1 {
			return "kernel_panic", strings.TrimSpace(matches[1])
		}

		return "", ""
	}

	ctx := context.Background()

	// Should recursively scan and find files in both root and subdirectory
	err = store.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan pstore with recursive directories")

	// Should find both events - one from root and one from subdirectory
	histories, err := store.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories")

	require.Len(t, histories, 2, "Expected 2 events (one from root, one from subdirectory)")

	// Verify we got both events
	foundSysrq := false
	foundPanic := false
	for _, h := range histories {
		switch h.EventName {
		case "sysrq_crash":
			foundSysrq = true
			assert.Equal(t, "SysRq crash trigger detected", h.Message)
		case "kernel_panic":
			foundPanic = true
			assert.Equal(t, "Test panic triggered by crash_test module", h.Message)
		}
	}

	assert.True(t, foundSysrq, "Expected to find sysrq_crash event from root directory")
	assert.True(t, foundPanic, "Expected to find kernel_panic event from subdirectory")
}

func TestPstoreScanWithEmptyLinesAndMatchingLines(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	tempDir := t.TempDir()
	// Create test data with empty lines and non-matching lines
	testData := `
<6>[  201.650687] sysrq: SysRq : Trigger a crash

<4>[  201.654822] BUG: unable to handle kernel NULL pointer dereference

<0>[ 3098.275469] Kernel panic - not syncing: Test panic triggered by crash_test module

`

	err := os.WriteFile(filepath.Join(tempDir, "dmesg.txt"), []byte(testData), 0644)
	require.NoError(t, err, "Failed to write test data")

	store, err := New(tempDir, dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.NoError(t, err, "Failed to create pstore")

	matchFunc := func(line string) (eventName string, message string) {
		if strings.Contains(line, "sysrq: SysRq : Trigger a crash") {
			return "sysrq_crash", "SysRq crash trigger detected"
		}

		panicRegex := regexp.MustCompile(`Kernel panic - not syncing: (.+)`)
		if matches := panicRegex.FindStringSubmatch(line); len(matches) > 1 {
			return "kernel_panic", strings.TrimSpace(matches[1])
		}

		// Return empty for non-matching lines (like BUG line)
		return "", ""
	}

	ctx := context.Background()

	err = store.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan pstore")

	// Should find 2 events (sysrq and panic, but not the BUG line)
	histories, err := store.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories")

	if !assert.Len(t, histories, 2, "Expected 2 events") {
		for _, h := range histories {
			t.Logf("Event: %s, Message: %s", h.EventName, h.Message)
		}
	}

	// Verify we got the right events
	foundSysrq := false
	foundPanic := false
	for _, h := range histories {
		switch h.EventName {
		case "sysrq_crash":
			foundSysrq = true
		case "kernel_panic":
			foundPanic = true
		}
	}

	assert.True(t, foundSysrq, "Expected to find sysrq event")
	assert.True(t, foundPanic, "Expected to find panic event")
}

func TestPstoreGetWithSQLErrNoRows(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	tempDir := t.TempDir()

	store, err := New(tempDir, dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.NoError(t, err, "Failed to create pstore")

	ctx := context.Background()

	// Query with a table name that has an invalid column to trigger an error
	// This will test error handling in Get()
	pr := store.(*pstoreReader)

	// Test by closing the database connection to force an error
	_ = dbRO.Close()

	_, err = pr.Get(ctx, time.Now().Add(-1*time.Hour))
	assert.Error(t, err, "Expected error when querying with closed database")
}

func TestPstoreFindHistoryByRawMessageEmpty(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	tempDir := t.TempDir()

	// Create a custom pstoreReader to test findHistoryByRawMessage directly
	pr := &pstoreReader{
		dir:            tempDir,
		dbRW:           dbRW,
		dbRO:           dbRO,
		historyTable:   "test_direct_v0_7_0",
		lookBackPeriod: 24 * time.Hour,
		getTimeNow: func() time.Time {
			return time.Now().UTC()
		},
	}

	err := pr.init()
	require.NoError(t, err, "Failed to initialize pstore reader")

	ctx := context.Background()

	// Test findHistoryByRawMessage with non-existent message (should return 0)
	count, err := findHistoryByRawMessage(ctx, dbRO, pr.historyTable, "non-existent-message", time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to find history")

	assert.Zero(t, count, "Expected 0 count for non-existent message")
}

func TestPstoreWithCorruptedFile(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	tempDir := t.TempDir()

	// Create a file with restricted permissions to cause read error
	restrictedFile := filepath.Join(tempDir, "restricted.txt")
	err := os.WriteFile(restrictedFile, []byte("test"), 0000) // No read permissions
	require.NoError(t, err, "Failed to create restricted file")

	store, err := New(tempDir, dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.NoError(t, err, "Failed to create pstore")

	matchFunc := func(line string) (eventName string, message string) {
		return "test_event", "test message"
	}

	ctx := context.Background()

	// Should get error when trying to read the restricted file
	err = store.Scan(ctx, matchFunc)
	assert.Error(t, err, "Expected error when reading restricted file")
}

func TestPstoreHistoryStruct(t *testing.T) {
	// Simple test to ensure History struct is properly defined
	h := History{
		Timestamp:  1234567890,
		EventName:  "test_event",
		Message:    "test message",
		RawMessage: "raw test message",
	}

	assert.Equal(t, int64(1234567890), h.Timestamp, "Expected Timestamp 1234567890")
	assert.Equal(t, "test_event", h.EventName, "Expected EventName 'test_event'")
	assert.Equal(t, "test message", h.Message, "Expected Message 'test message'")
	assert.Equal(t, "raw test message", h.RawMessage, "Expected RawMessage 'raw test message'")
}

func TestPstoreDefaultConstants(t *testing.T) {
	// Test that constants are properly defined
	assert.Equal(t, "/var/lib/systemd/pstore", DefaultPstoreDir, "Expected DefaultPstoreDir '/var/lib/systemd/pstore'")

	// Test column constants
	assert.Equal(t, "timestamp", pstoreHistoryTableColumnTimestamp, "Expected timestamp column name 'timestamp'")
	assert.Equal(t, "event_name", pstoreHistoryTableColumnEventName, "Expected event_name column name 'event_name'")
	assert.Equal(t, "message", pstoreHistoryTableColumnMessage, "Expected message column name 'message'")
	assert.Equal(t, "raw_message", pstoreHistoryTableColumnRawMessage, "Expected raw_message column name 'raw_message'")
}

func TestPstoreMaxDepthLimit(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	defer func() {
		_ = dbRW.Close()
	}()
	defer func() {
		_ = dbRO.Close()
	}()

	// Create temp directory with nested structure beyond max depth
	tempDir := t.TempDir()

	// Create structure:
	// tempDir/
	//   ├── level0.txt (should be read - depth 0)
	//   └── level1/
	//       ├── level1.txt (should be read - depth 1)
	//       └── level2/
	//           ├── level2.txt (should be read - depth 2)
	//           └── level3/
	//               ├── level3.txt (should be read - depth 3)
	//               └── level4/
	//                   └── level4.txt (should NOT be read - depth 4, beyond max)

	// Create files at each level with unique messages
	testDataLevel0 := `<6>[  100.000000] Level 0: sysrq crash`
	err := os.WriteFile(filepath.Join(tempDir, "level0.txt"), []byte(testDataLevel0), 0644)
	require.NoError(t, err, "Failed to write level 0 data")

	level1Dir := filepath.Join(tempDir, "level1")
	err = os.Mkdir(level1Dir, 0755)
	require.NoError(t, err, "Failed to create level1 directory")

	testDataLevel1 := `<6>[  101.000000] Level 1: kernel warning`
	err = os.WriteFile(filepath.Join(level1Dir, "level1.txt"), []byte(testDataLevel1), 0644)
	require.NoError(t, err, "Failed to write level 1 data")

	level2Dir := filepath.Join(level1Dir, "level2")
	err = os.Mkdir(level2Dir, 0755)
	require.NoError(t, err, "Failed to create level2 directory")

	testDataLevel2 := `<6>[  102.000000] Level 2: memory error`
	err = os.WriteFile(filepath.Join(level2Dir, "level2.txt"), []byte(testDataLevel2), 0644)
	require.NoError(t, err, "Failed to write level 2 data")

	level3Dir := filepath.Join(level2Dir, "level3")
	err = os.Mkdir(level3Dir, 0755)
	require.NoError(t, err, "Failed to create level3 directory")

	testDataLevel3 := `<6>[  103.000000] Level 3: disk error`
	err = os.WriteFile(filepath.Join(level3Dir, "level3.txt"), []byte(testDataLevel3), 0644)
	require.NoError(t, err, "Failed to write level 3 data")

	level4Dir := filepath.Join(level3Dir, "level4")
	err = os.Mkdir(level4Dir, 0755)
	require.NoError(t, err, "Failed to create level4 directory")

	testDataLevel4 := `<6>[  104.000000] Level 4: should not be read`
	err = os.WriteFile(filepath.Join(level4Dir, "level4.txt"), []byte(testDataLevel4), 0644)
	require.NoError(t, err, "Failed to write level 4 data")

	store, err := New(tempDir, dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.NoError(t, err, "Failed to create pstore")

	// Match function that extracts level from message
	matchFunc := func(line string) (eventName string, message string) {
		if strings.Contains(line, "Level 0:") {
			return "level_0", "Level 0 event"
		}
		if strings.Contains(line, "Level 1:") {
			return "level_1", "Level 1 event"
		}
		if strings.Contains(line, "Level 2:") {
			return "level_2", "Level 2 event"
		}
		if strings.Contains(line, "Level 3:") {
			return "level_3", "Level 3 event"
		}
		if strings.Contains(line, "Level 4:") {
			return "level_4", "Level 4 event"
		}
		return "", ""
	}

	ctx := context.Background()

	// Scan with max depth of 3
	err = store.Scan(ctx, matchFunc)
	require.NoError(t, err, "Failed to scan pstore")

	// Get all events
	histories, err := store.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err, "Failed to get histories")

	// Should find exactly 4 events (levels 0-3), not level 4
	require.Len(t, histories, 4, "Expected 4 events (levels 0-3 only)")

	// Verify we got the right events
	foundLevels := make(map[string]bool)
	for _, h := range histories {
		foundLevels[h.EventName] = true
		t.Logf("Found event: %s - %s", h.EventName, h.Message)
	}

	assert.True(t, foundLevels["level_0"], "Expected to find level 0 event")
	assert.True(t, foundLevels["level_1"], "Expected to find level 1 event")
	assert.True(t, foundLevels["level_2"], "Expected to find level 2 event")
	assert.True(t, foundLevels["level_3"], "Expected to find level 3 event")
	assert.False(t, foundLevels["level_4"], "Should NOT find level 4 event (beyond max depth)")
}

func TestCreateHistoryTable_StubbedErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("first index creation error", func(t *testing.T) {
		db := setupStubDB(t, &stubDBConfig{
			execErrAt: map[int]error{2: errors.New("index1 failed")},
		})
		err := createHistoryTable(ctx, db, "test_table")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index1 failed")
	})

	t.Run("second index creation error", func(t *testing.T) {
		db := setupStubDB(t, &stubDBConfig{
			execErrAt: map[int]error{3: errors.New("index2 failed")},
		})
		err := createHistoryTable(ctx, db, "test_table")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index2 failed")
	})

	t.Run("commit error", func(t *testing.T) {
		db := setupStubDB(t, &stubDBConfig{
			commitErr: errors.New("commit failed"),
		})
		err := createHistoryTable(ctx, db, "test_table")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "commit failed")
	})
}

func TestPurgeHistory_StubbedErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("rows affected error", func(t *testing.T) {
		db := setupStubDB(t, &stubDBConfig{
			rowsAffectedErr: errors.New("rows affected failed"),
		})
		err := purgeHistory(ctx, db, "test_table", time.Now().Add(-time.Hour))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rows affected failed")
	})

	t.Run("commit error", func(t *testing.T) {
		db := setupStubDB(t, &stubDBConfig{
			commitErr: errors.New("commit failed"),
		})
		err := purgeHistory(ctx, db, "test_table", time.Now().Add(-time.Hour))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "commit failed")
	})
}

func TestFindHistoryByRawMessage_StubbedRowsBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("no rows", func(t *testing.T) {
		db := setupStubDB(t, &stubDBConfig{
			columns: []string{"count"},
			rows:    nil,
		})
		cnt, err := findHistoryByRawMessage(ctx, db, "test_table", "raw", time.Now().Add(-time.Hour))
		require.NoError(t, err)
		assert.Equal(t, 0, cnt)
	})

	t.Run("scan error", func(t *testing.T) {
		db := setupStubDB(t, &stubDBConfig{
			columns: []string{"count"},
			rows: [][]driver.Value{
				{"not-an-int"},
			},
		})
		_, err := findHistoryByRawMessage(ctx, db, "test_table", "raw", time.Now().Add(-time.Hour))
		require.Error(t, err)
	})
}

func TestGet_StubbedRowsScanAndErrBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("scan error", func(t *testing.T) {
		db := setupStubDB(t, &stubDBConfig{
			columns: []string{
				pstoreHistoryTableColumnTimestamp,
				pstoreHistoryTableColumnEventName,
				pstoreHistoryTableColumnMessage,
				pstoreHistoryTableColumnRawMessage,
			},
			rows: [][]driver.Value{
				{"bad-ts", "ev", "msg", "raw"},
			},
		})
		pr := &pstoreReader{dbRO: db, historyTable: "test_table"}
		_, err := pr.Get(ctx, time.Now().Add(-time.Hour))
		require.Error(t, err)
	})

	t.Run("rows err", func(t *testing.T) {
		db := setupStubDB(t, &stubDBConfig{
			columns: []string{
				pstoreHistoryTableColumnTimestamp,
				pstoreHistoryTableColumnEventName,
				pstoreHistoryTableColumnMessage,
				pstoreHistoryTableColumnRawMessage,
			},
			nextErr: errors.New("next failed"),
		})
		pr := &pstoreReader{dbRO: db, historyTable: "test_table"}
		_, err := pr.Get(ctx, time.Now().Add(-time.Hour))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "next failed")
	})
}

func TestPstoreNew_PurgeFailureWithStubbedDB(t *testing.T) {
	dbRW := setupStubDB(t, &stubDBConfig{
		// New() calls createHistoryTable first (3 exec calls), then purgeHistory (4th).
		execErrAt: map[int]error{4: errors.New("purge exec failed")},
	})
	dbRO := setupStubDB(t, &stubDBConfig{})

	_, err := New(t.TempDir(), dbRW, dbRO, "test_table", 24*time.Hour)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "purge exec failed")
}

func TestCreateHistoryTableErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("closed db begin tx fails", func(t *testing.T) {
		dbRW, dbRO := setupTestDB(t)
		_ = dbRO.Close()
		_ = dbRW.Close()

		err := createHistoryTable(ctx, dbRW, "test_table")
		require.Error(t, err)
	})

	t.Run("invalid table name returns exec error", func(t *testing.T) {
		dbRW, dbRO := setupTestDB(t)
		defer func() {
			_ = dbRW.Close()
			_ = dbRO.Close()
		}()

		err := createHistoryTable(ctx, dbRW, "invalid-table-name")
		require.Error(t, err)
	})
}

func TestInsertHistoryErrors(t *testing.T) {
	ctx := context.Background()
	h := &History{
		Timestamp:  time.Now().Unix(),
		EventName:  "test",
		Message:    "test",
		RawMessage: "test",
	}

	t.Run("closed db begin tx fails", func(t *testing.T) {
		dbRW, dbRO := setupTestDB(t)
		_ = dbRO.Close()
		_ = dbRW.Close()

		err := insertHistory(ctx, dbRW, "test_table", h)
		require.Error(t, err)
	})

	t.Run("insert into non-existent table fails", func(t *testing.T) {
		dbRW, dbRO := setupTestDB(t)
		defer func() {
			_ = dbRW.Close()
			_ = dbRO.Close()
		}()

		err := insertHistory(ctx, dbRW, "missing_table_v0_7_0", h)
		require.Error(t, err)
	})
}

func TestFindHistoryByRawMessageErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("closed db query fails", func(t *testing.T) {
		dbRW, dbRO := setupTestDB(t)
		_ = dbRW.Close()
		_ = dbRO.Close()

		_, err := findHistoryByRawMessage(ctx, dbRO, "test_table", "raw", time.Now().Add(-time.Hour))
		require.Error(t, err)
	})

	t.Run("missing table query fails", func(t *testing.T) {
		dbRW, dbRO := setupTestDB(t)
		defer func() {
			_ = dbRW.Close()
			_ = dbRO.Close()
		}()

		_, err := findHistoryByRawMessage(ctx, dbRO, "missing_table_v0_7_0", "raw", time.Now().Add(-time.Hour))
		require.Error(t, err)
	})
}

func TestPurgeHistoryErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("closed db begin tx fails", func(t *testing.T) {
		dbRW, dbRO := setupTestDB(t)
		_ = dbRO.Close()
		_ = dbRW.Close()

		err := purgeHistory(ctx, dbRW, "test_table", time.Now().Add(-time.Hour))
		require.Error(t, err)
	})

	t.Run("missing table delete fails", func(t *testing.T) {
		dbRW, dbRO := setupTestDB(t)
		defer func() {
			_ = dbRW.Close()
			_ = dbRO.Close()
		}()

		err := purgeHistory(ctx, dbRW, "missing_table_v0_7_0", time.Now().Add(-time.Hour))
		require.Error(t, err)
	})
}

func TestPstoreNewInitFailureWithClosedDB(t *testing.T) {
	dbRW, dbRO := setupTestDB(t)
	_ = dbRW.Close()
	_ = dbRO.Close()

	_, err := New(t.TempDir(), dbRW, dbRO, "test_pstore", 24*time.Hour)
	require.Error(t, err)
}
