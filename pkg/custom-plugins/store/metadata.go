package store

import (
	"context"
	"database/sql"
	"encoding/json"

	customplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/metadata"
)

// SaveSpecs saves the custom plugin specs to the metadata,
// if and only if the specs are different from the previous ones.
// It returns true, if the record is inserted or updated.
// It returns false, if the record is not updated.
func SaveSpecs(ctx context.Context, dbRW *sql.DB, specs customplugins.Specs) (bool, error) {
	prev, err := metadata.ReadMetadata(ctx, dbRW, metadata.MetadataKeyPluginsSpec)
	if err != nil {
		return false, err
	}

	cur, err := json.Marshal(specs)
	if err != nil {
		return false, err
	}

	needUpdate := prev == "" || prev != string(cur)
	if !needUpdate {
		// record is not updated, no need to save
		// thus returning false
		return false, nil
	}

	log.Logger.Infow("saving plugin specs to metadata", "count", len(specs))
	err = metadata.SetMetadata(ctx, dbRW, metadata.MetadataKeyPluginsSpec, string(cur))
	if err != nil {
		return false, err
	}

	log.Logger.Infow("successfully saved plugin specs to metadata", "count", len(specs))

	// record is updated, returning true
	return true, nil
}

// LoadSpecs loads the custom plugin specs from the metadata.
// If the metadata is not found, it returns an empty string and no error.
// If the metadata is found but empty, it returns an empty specs.
// If the metadata is found and not empty, it returns the specs.
func LoadSpecs(ctx context.Context, dbRO *sql.DB) (customplugins.Specs, error) {
	prev, err := metadata.ReadMetadata(ctx, dbRO, metadata.MetadataKeyPluginsSpec)
	if err != nil {
		return nil, err
	}

	if prev == "" {
		return nil, nil
	}

	var specs customplugins.Specs
	if err := json.Unmarshal([]byte(prev), &specs); err != nil {
		return nil, err
	}

	log.Logger.Infow("successfully loaded plugin specs from metadata", "count", len(specs))
	return specs, nil
}
