package session

import (
	"context"
	"fmt"
	"net/http/cookiejar"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
)

// processUpdateToken updates the session token in the database with the new token from the control plane.
// It validates the new token before updating, and immediately reconnects the session with the new token.
func (s *Session) processUpdateToken(payload Request, response *Response) {
	if payload.Token == "" {
		response.Error = "token cannot be empty"
		log.Logger.Errorw("updateToken request with empty token")
		return
	}

	// if in-memory cached token equal update token, just return is ok
	cacheToken := s.getToken()
	if cacheToken == payload.Token {
		log.Logger.Info("cached token already matches the requested token")
		return
	}

	if s.dbRW == nil {
		response.Error = "database connection not available"
		log.Logger.Errorw("updateToken failed: database connection is nil")
		return
	}

	// Validate the new token before updating
	log.Logger.Infow("validating new token with control plane")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.validateTokenWithHealthCheck(ctx, payload.Token); err != nil {
		response.Error = fmt.Sprintf("token validation failed: %v", err)
		log.Logger.Errorw("new token validation failed", "error", err)
		return
	}
	log.Logger.Infow("new token validated successfully")

	// Save the new token to database
	if err := pkgmetadata.SetMetadata(ctx, s.dbRW, pkgmetadata.MetadataKeyToken, payload.Token); err != nil {
		response.Error = err.Error()
		log.Logger.Errorw("failed to update token in database", "error", err)
		return
	}

	// Update in-memory token
	s.setToken(payload.Token)

	log.Logger.Infow("token updated successfully in database and memory")

	// Immediately close the current session to force reconnection with the new token
	log.Logger.Infow("closing current session to immediately reconnect with new token")
	go func() {
		// Give some time for the response to be sent back to control plane
		time.Sleep(2 * time.Second)
		s.closer.Close()
	}()
}

// validateTokenWithHealthCheck validates the new token using the checkServerHealth mechanism
func (s *Session) validateTokenWithHealthCheck(ctx context.Context, newToken string) error {
	// Create a new cookie jar for validation
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("failed to create cookie jar: %w", err)
	}

	// Use the existing checkServerHealth function to validate the token
	// Pass newToken directly to avoid modifying the session's current token
	if err := s.checkServerHealthFunc(ctx, jar, newToken); err != nil {
		return fmt.Errorf("health check with new token failed: %w", err)
	}

	return nil
}

// processGetToken retrieves the current token value from the database.
func (s *Session) processGetToken(response *Response) {
	if s.dbRO == nil {
		response.Error = "database connection not available"
		log.Logger.Errorw("getToken failed: database connection is nil")
		return
	}

	// if session token cache is exist, just return it
	tokenCache := s.getToken()
	if tokenCache != "" {
		response.Token = tokenCache
		return
	}

	// else, get token from metadata
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	token, err := pkgmetadata.ReadToken(ctx, s.dbRO)
	if err != nil {
		response.Error = err.Error()
		log.Logger.Errorw("failed to read token from database", "error", err)
		return
	}
	s.setToken(token)
	response.Token = token
	log.Logger.Infow("token retrieved successfully from database")
}
