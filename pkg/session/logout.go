package session

import (
	"context"

	"github.com/leptonai/gpud/pkg/config"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// processLogout handles the logout request
func (s *Session) processLogout(ctx context.Context, response *Response) {
	stateFile := config.StateFilePath(s.dataDir)

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		log.Logger.Errorw("failed to open state file", "error", err)
		response.Error = err.Error()
		dbRW.Close()
		return
	}
	if err = pkgmetadata.DeleteAllMetadata(ctx, dbRW); err != nil {
		log.Logger.Errorw("failed to purge metadata", "error", err)
		response.Error = err.Error()
		dbRW.Close()
		return
	}
	dbRW.Close()
	err = pkghost.Stop(s.ctx, pkghost.WithDelaySeconds(10))
	if err != nil {
		log.Logger.Errorf("failed to trigger stop gpud: %v", err)
		response.Error = err.Error()
	}
}
