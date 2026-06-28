package run

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgsqlite "github.com/leptonai/gpud/pkg/sqlite"
)

func TestPersistMetadataOverridesErrors(t *testing.T) {
	t.Run("state open", func(t *testing.T) {
		mockey.PatchConvey("metadata override state open error", t, func() {
			mockey.Mock(pkgsqlite.Open).To(func(string, ...pkgsqlite.OpOption) (*sql.DB, error) {
				return nil, errors.New("open failed")
			}).Build()

			err := persistMetadataOverrides(context.Background(), "/tmp/state.db", "", "machine-id", false, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to open state for metadata overrides")
		})
	})

	t.Run("metadata table", func(t *testing.T) {
		mockey.PatchConvey("metadata override table error", t, func() {
			mockey.Mock(pkgsqlite.Open).To(func(string, ...pkgsqlite.OpOption) (*sql.DB, error) {
				return &sql.DB{}, nil
			}).Build()
			mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error { return nil }).Build()
			mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(context.Context, *sql.DB) error {
				return errors.New("create table failed")
			}).Build()

			err := persistMetadataOverrides(context.Background(), "/tmp/state.db", "", "machine-id", false, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to ensure metadata table")
		})
	})

	t.Run("endpoint write", func(t *testing.T) {
		mockey.PatchConvey("metadata override endpoint write error", t, func() {
			mockey.Mock(pkgsqlite.Open).To(func(string, ...pkgsqlite.OpOption) (*sql.DB, error) {
				return &sql.DB{}, nil
			}).Build()
			mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error { return nil }).Build()
			mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(context.Context, *sql.DB) error { return nil }).Build()
			mockey.Mock(pkgmetadata.SetMetadata).To(func(context.Context, *sql.DB, string, string) error {
				return errors.New("endpoint write failed")
			}).Build()

			err := persistMetadataOverrides(context.Background(), "/tmp/state.db", "endpoint", "", false, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to set endpoint metadata")
		})
	})

	t.Run("machine id read", func(t *testing.T) {
		mockey.PatchConvey("metadata override machine id read error", t, func() {
			mockey.Mock(pkgsqlite.Open).To(func(string, ...pkgsqlite.OpOption) (*sql.DB, error) {
				return &sql.DB{}, nil
			}).Build()
			mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error { return nil }).Build()
			mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(context.Context, *sql.DB) error { return nil }).Build()
			mockey.Mock(pkgmetadata.ReadMetadata).To(func(context.Context, *sql.DB, string) (string, error) {
				return "", errors.New("machine id read failed")
			}).Build()

			err := persistMetadataOverrides(context.Background(), "/tmp/state.db", "", "machine-id", false, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read persisted machine-id")
		})
	})

	t.Run("machine id write", func(t *testing.T) {
		mockey.PatchConvey("metadata override machine id write error", t, func() {
			mockey.Mock(pkgsqlite.Open).To(func(string, ...pkgsqlite.OpOption) (*sql.DB, error) {
				return &sql.DB{}, nil
			}).Build()
			mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error { return nil }).Build()
			mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(context.Context, *sql.DB) error { return nil }).Build()
			mockey.Mock(pkgmetadata.ReadMetadata).To(func(context.Context, *sql.DB, string) (string, error) {
				return "", nil
			}).Build()
			mockey.Mock(pkgmetadata.SetMetadata).To(func(context.Context, *sql.DB, string, string) error {
				return errors.New("machine id write failed")
			}).Build()

			err := persistMetadataOverrides(context.Background(), "/tmp/state.db", "", "machine-id", false, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to set machine-id metadata")
		})
	})
}
