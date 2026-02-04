package metadata

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetMetadata_InsertExecErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("SetMetadata insert exec error", t, func() {
		mockey.Mock(ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil
		}).Build()
		mockey.Mock((*sql.DB).ExecContext).To(func(_ *sql.DB, _ context.Context, _ string, _ ...any) (sql.Result, error) {
			return nil, errors.New("exec failed")
		}).Build()

		err := SetMetadata(context.Background(), &sql.DB{}, "key", "value")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exec failed")
	})
}

func TestReadMetadata_NoRowsWithMockey(t *testing.T) {
	mockey.PatchConvey("ReadMetadata handles no rows", t, func() {
		mockey.Mock((*sql.DB).QueryRowContext).To(func(_ *sql.DB, _ context.Context, _ string, _ ...any) *sql.Row {
			return &sql.Row{}
		}).Build()
		mockey.Mock((*sql.Row).Scan).To(func(_ *sql.Row, _ ...any) error {
			return sql.ErrNoRows
		}).Build()

		val, err := ReadMetadata(context.Background(), &sql.DB{}, "key")
		require.NoError(t, err)
		assert.Empty(t, val)
	})
}

func TestReadAllMetadata_QueryErrorsWithMockey(t *testing.T) {
	mockey.PatchConvey("ReadAllMetadata handles sql.ErrNoRows", t, func() {
		mockey.Mock((*sql.DB).QueryContext).To(func(_ *sql.DB, _ context.Context, _ string, _ ...any) (*sql.Rows, error) {
			return nil, sql.ErrNoRows
		}).Build()

		data, err := ReadAllMetadata(context.Background(), &sql.DB{})
		require.NoError(t, err)
		assert.Nil(t, data)
	})

	mockey.PatchConvey("ReadAllMetadata handles query error", t, func() {
		mockey.Mock((*sql.DB).QueryContext).To(func(_ *sql.DB, _ context.Context, _ string, _ ...any) (*sql.Rows, error) {
			return nil, errors.New("query failed")
		}).Build()

		data, err := ReadAllMetadata(context.Background(), &sql.DB{})
		require.Error(t, err)
		assert.Nil(t, data)
	})
}

func TestReadAllMetadata_ScanErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("ReadAllMetadata returns scan error", t, func() {
		mockey.Mock((*sql.DB).QueryContext).To(func(_ *sql.DB, _ context.Context, _ string, _ ...any) (*sql.Rows, error) {
			return &sql.Rows{}, nil
		}).Build()

		var nextCalls int
		mockey.Mock((*sql.Rows).Next).To(func(_ *sql.Rows) bool {
			nextCalls++
			return nextCalls == 1
		}).Build()
		mockey.Mock((*sql.Rows).Close).To(func(_ *sql.Rows) error {
			return nil
		}).Build()
		mockey.Mock((*sql.Rows).Scan).To(func(_ *sql.Rows, _ ...any) error {
			return errors.New("scan failed")
		}).Build()

		data, err := ReadAllMetadata(context.Background(), &sql.DB{})
		require.Error(t, err)
		assert.Nil(t, data)
	})
}
