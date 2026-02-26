package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_SQLOpenErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("Open returns sql.Open error", t, func() {
		mockey.Mock(sql.Open).To(func(driverName, dataSourceName string) (*sql.DB, error) {
			return nil, errors.New("open failed")
		}).Build()

		db, err := Open("/tmp/test.db")
		require.Error(t, err)
		assert.Nil(t, db)
		assert.Contains(t, err.Error(), "failed to open sqlite3 database")
	})
}

func TestReadDBSize_ClosedDB(t *testing.T) {
	db, _ := Open(":memory:")
	require.NoError(t, db.Close())

	_, err := ReadDBSize(context.Background(), db)
	require.Error(t, err)
}

func TestTableExists_ClosedDB(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = TableExists(context.Background(), db, "table")
	require.Error(t, err)
}

func TestBuildConnectionString_ApplyOptsError(t *testing.T) {
	mockey.PatchConvey("BuildConnectionString handles applyOpts error", t, func() {
		mockey.Mock((*Op).applyOpts).To(func(*Op, []OpOption) error {
			return errors.New("applyOpts error")
		}).Build()

		_, err := BuildConnectionString(":memory:")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "applyOpts error")
	})
}

func TestOpen_ApplyOptsError(t *testing.T) {
	mockey.PatchConvey("Open handles applyOpts error", t, func() {
		mockey.Mock((*Op).applyOpts).To(func(*Op, []OpOption) error {
			return errors.New("applyOpts error")
		}).Build()

		db, err := Open(":memory:")
		require.Error(t, err)
		assert.Nil(t, db)
		assert.Contains(t, err.Error(), "applyOpts error")
	})
}

func TestRunCompact_OpenRWError(t *testing.T) {
	mockey.PatchConvey("RunCompact handles Open RW error", t, func() {
		mockey.Mock(Open).To(func(file string, opts ...OpOption) (*sql.DB, error) {
			return nil, errors.New("open rw error")
		}).Build()

		err := RunCompact(context.Background(), "/tmp/test.db")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "open rw error")
	})
}

func TestRunCompact_OpenROError(t *testing.T) {
	mockey.PatchConvey("RunCompact handles Open RO error", t, func() {
		var calls int
		mockey.Mock(Open).To(func(file string, opts ...OpOption) (*sql.DB, error) {
			calls++
			if calls == 2 {
				return nil, errors.New("open ro error")
			}
			db, _ := sql.Open("sqlite3", ":memory:")
			return db, nil
		}).Build()

		err := RunCompact(context.Background(), "/tmp/test.db")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "open ro error")
	})
}

func TestRunCompact_ReadDBSizeBeforeError(t *testing.T) {
	mockey.PatchConvey("RunCompact handles ReadDBSize error before compact", t, func() {
		mockey.Mock(ReadDBSize).To(func(ctx context.Context, dbRO *sql.DB) (uint64, error) {
			return 0, errors.New("read size error")
		}).Build()

		mockey.Mock(Open).To(func(file string, opts ...OpOption) (*sql.DB, error) {
			db, _ := sql.Open("sqlite3", ":memory:")
			return db, nil
		}).Build()

		err := RunCompact(context.Background(), "/tmp/test.db")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read size error")
	})
}

func TestRunCompact_CompactError(t *testing.T) {
	mockey.PatchConvey("RunCompact handles Compact error", t, func() {
		mockey.Mock(Compact).To(func(ctx context.Context, db *sql.DB) error {
			return errors.New("compact error")
		}).Build()

		mockey.Mock(Open).To(func(file string, opts ...OpOption) (*sql.DB, error) {
			db, _ := sql.Open("sqlite3", ":memory:")
			return db, nil
		}).Build()

		mockey.Mock(ReadDBSize).To(func(ctx context.Context, dbRO *sql.DB) (uint64, error) {
			return 100, nil
		}).Build()

		err := RunCompact(context.Background(), "/tmp/test.db")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compact error")
	})
}

func TestRunCompact_ReadDBSizeAfterError(t *testing.T) {
	mockey.PatchConvey("RunCompact handles ReadDBSize error after compact", t, func() {
		var calls int
		mockey.Mock(ReadDBSize).To(func(ctx context.Context, dbRO *sql.DB) (uint64, error) {
			calls++
			if calls == 2 {
				return 0, errors.New("read size error 2")
			}
			return 100, nil
		}).Build()

		mockey.Mock(Compact).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(Open).To(func(file string, opts ...OpOption) (*sql.DB, error) {
			db, _ := sql.Open("sqlite3", ":memory:")
			return db, nil
		}).Build()

		err := RunCompact(context.Background(), "/tmp/test.db")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read size error 2")
	})
}

func TestOpenTestDB_CreateTempError(t *testing.T) {
	if os.Getenv("BE_CRASHER_CREATE_TEMP") == "1" {
		t.Setenv("TMPDIR", "/nonexistent/path/for/test/that/does/not/exist")
		OpenTestDB(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestOpenTestDB_CreateTempError")
	cmd.Env = append(os.Environ(), "BE_CRASHER_CREATE_TEMP=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	}
	t.Fatalf("process ran with err %v, want exit status 1", err)
}

func TestOpenTestDB_OpenRWError(t *testing.T) {
	if os.Getenv("BE_CRASHER_OPEN_RW") == "1" {
		mockey.Mock(Open).To(func(file string, opts ...OpOption) (*sql.DB, error) {
			return nil, errors.New("open rw error")
		}).Build()
		OpenTestDB(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestOpenTestDB_OpenRWError")
	cmd.Env = append(os.Environ(), "BE_CRASHER_OPEN_RW=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	}
	t.Fatalf("process ran with err %v, want exit status 1", err)
}

func TestOpenTestDB_OpenROError(t *testing.T) {
	if os.Getenv("BE_CRASHER_OPEN_RO") == "1" {
		var calls int
		mockey.Mock(Open).To(func(file string, opts ...OpOption) (*sql.DB, error) {
			calls++
			if calls == 2 {
				return nil, errors.New("open ro error")
			}
			db, _ := sql.Open("sqlite3", ":memory:")
			return db, nil
		}).Build()
		OpenTestDB(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestOpenTestDB_OpenROError")
	cmd.Env = append(os.Environ(), "BE_CRASHER_OPEN_RO=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	}
	t.Fatalf("process ran with err %v, want exit status 1", err)
}
