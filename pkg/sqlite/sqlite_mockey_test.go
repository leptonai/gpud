package sqlite

import (
	"context"
	"database/sql"
	"errors"
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

func TestReadDBSize_NoPageCountWithMockey(t *testing.T) {
	mockey.PatchConvey("ReadDBSize handles no page count", t, func() {
		mockey.Mock((*sql.DB).QueryRowContext).To(func(_ *sql.DB, _ context.Context, _ string, _ ...any) *sql.Row {
			return &sql.Row{}
		}).Build()
		mockey.Mock((*sql.Row).Scan).To(func(_ *sql.Row, _ ...any) error {
			return sql.ErrNoRows
		}).Build()

		_, err := ReadDBSize(context.Background(), &sql.DB{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no page count")
	})
}

func TestReadDBSize_NoPageSizeWithMockey(t *testing.T) {
	mockey.PatchConvey("ReadDBSize handles no page size", t, func() {
		mockey.Mock((*sql.DB).QueryRowContext).To(func(_ *sql.DB, _ context.Context, _ string, _ ...any) *sql.Row {
			return &sql.Row{}
		}).Build()

		var calls int
		mockey.Mock((*sql.Row).Scan).To(func(_ *sql.Row, _ ...any) error {
			calls++
			if calls == 1 {
				return nil
			}
			return sql.ErrNoRows
		}).Build()

		_, err := ReadDBSize(context.Background(), &sql.DB{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no page size")
	})
}

func TestTableExists_NoRowsWithMockey(t *testing.T) {
	mockey.PatchConvey("TableExists handles no rows", t, func() {
		mockey.Mock((*sql.DB).QueryRowContext).To(func(_ *sql.DB, _ context.Context, _ string, _ ...any) *sql.Row {
			return &sql.Row{}
		}).Build()
		mockey.Mock((*sql.Row).Scan).To(func(_ *sql.Row, _ ...any) error {
			return sql.ErrNoRows
		}).Build()

		exists, err := TableExists(context.Background(), &sql.DB{}, "table")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}
