package metadata

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestReadMachineID(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	require.NoError(t, CreateTableMetadata(ctx, dbRW))

	machineID, err := ReadMachineID(ctx, dbRO)
	require.NoError(t, err)
	assert.Empty(t, machineID)

	testMachineID := "test-machine-id"
	require.NoError(t, SetMetadata(ctx, dbRW, MetadataKeyMachineID, testMachineID))

	machineID, err = ReadMachineID(ctx, dbRO)
	require.NoError(t, err)
	assert.Equal(t, testMachineID, machineID)

	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()
	_, err = ReadMachineID(canceledCtx, dbRO)
	assert.Error(t, err)
}

func TestReadToken(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	require.NoError(t, CreateTableMetadata(ctx, dbRW))

	token, err := ReadToken(ctx, dbRO)
	require.NoError(t, err)
	assert.Empty(t, token)

	testToken := "test-token"
	require.NoError(t, SetMetadata(ctx, dbRW, MetadataKeyToken, testToken))

	token, err = ReadToken(ctx, dbRO)
	require.NoError(t, err)
	assert.Equal(t, testToken, token)

	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()
	_, err = ReadToken(canceledCtx, dbRO)
	assert.Error(t, err)
}

func TestDeleteAllMetadata(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	require.NoError(t, CreateTableMetadata(ctx, dbRW))
	require.NoError(t, SetMetadata(ctx, dbRW, MetadataKeyMachineID, "test-machine-id"))
	require.NoError(t, SetMetadata(ctx, dbRW, MetadataKeyToken, "test-token"))

	var count int
	require.NoError(t, dbRO.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableNameGPUdMetadata)).Scan(&count))
	assert.Equal(t, 2, count)

	require.NoError(t, DeleteAllMetadata(ctx, dbRW))
	require.NoError(t, dbRO.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableNameGPUdMetadata)).Scan(&count))
	assert.Equal(t, 0, count)

	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()
	assert.Error(t, DeleteAllMetadata(canceledCtx, dbRW))
}

func TestMaskToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name:     "empty token",
			token:    "",
			expected: "...",
		},
		{
			name:     "very short token",
			token:    "short",
			expected: "...",
		},
		{
			name:     "token with nvapi-stg- prefix and sufficient length",
			token:    "nvapi-stg-abcdefghijklmnop1234",
			expected: "nvapi-stg-abcd...1234",
		},
		{
			name:     "token with nvapi- prefix and sufficient length",
			token:    "nvapi-abcdefghijklmnop1234",
			expected: "nvapi-abcd...1234",
		},
		{
			name:     "token without prefix and sufficient length",
			token:    "abcdefghijklmnopqrstuvwxyz1234",
			expected: "abcd...1234",
		},
		{
			name:     "token with nvapi-stg- prefix but trimmed part too short",
			token:    "nvapi-stg-short",
			expected: "nvapi-stg-...",
		},
		{
			name:     "token with nvapi- prefix but trimmed part too short",
			token:    "nvapi-short",
			expected: "nvapi-...",
		},
		{
			name:     "token with nvapi-stg- prefix and exactly 10 chars after prefix",
			token:    "nvapi-stg-1234567890",
			expected: "nvapi-stg-1234...7890",
		},
		{
			name:     "token with nvapi- prefix and exactly 10 chars after prefix",
			token:    "nvapi-1234567890",
			expected: "nvapi-1234...7890",
		},
		{
			name:     "token with nvapi-stg- prefix and 9 chars after prefix",
			token:    "nvapi-stg-123456789",
			expected: "nvapi-stg-...",
		},
		{
			name:     "token with nvapi- prefix and 9 chars after prefix",
			token:    "nvapi-123456789",
			expected: "nvapi-...",
		},
		{
			name:     "token exactly 10 characters long",
			token:    "1234567890",
			expected: "1234...7890",
		},
		{
			name:     "token 9 characters long",
			token:    "123456789",
			expected: "...",
		},
		{
			name:     "token with special characters",
			token:    "nvapi-ABC123!@#$%^&*()_+abcdef",
			expected: "nvapi-ABC1...cdef",
		},
		{
			name:     "token with nvapi-stg- prefix only",
			token:    "nvapi-stg-",
			expected: "nvapi-stg-...",
		},
		{
			name:     "token with nvapi- prefix only",
			token:    "nvapi-",
			expected: "nvapi-...",
		},
		{
			name:     "token that looks like prefix but isn't complete",
			token:    "nvapi",
			expected: "...",
		},
		{
			name:     "token that partially matches nvapi-stg- prefix",
			token:    "nvapi-stg",
			expected: "nvapi-...",
		},
		{
			name:     "long token without recognized prefix",
			token:    "verylongtoken1234567890abcdefghijklmnop",
			expected: "very...mnop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskToken(tt.token)
			assert.Equal(t, tt.expected, result)
		})
	}
}
