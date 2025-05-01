package infiniband

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseIBStatus(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("testdata/ibstatus.39.0.a100.all.active.0")
	require.NoError(t, err)

	parsed, err := ParseIBStatus(string(input))
	require.NoError(t, err)

	require.NotEmpty(t, parsed)
	require.Equal(t, len(parsed), 9)

	for i := 0; i < 8; i++ {
		require.Equal(t, "mlx5_"+strconv.Itoa(i), parsed[i].Device)
		require.Equal(t, "4: ACTIVE", parsed[i].State)
		require.Equal(t, "5: LinkUp", parsed[i].PhysicalState)
		require.Equal(t, "200 Gb/sec (4X HDR)", parsed[i].Rate)
		require.Equal(t, "InfiniBand", parsed[i].LinkLayer)
	}

	require.Equal(t, "mlx5_8", parsed[8].Device)
	require.Equal(t, "4: ACTIVE", parsed[8].State)
	require.Equal(t, "5: LinkUp", parsed[8].PhysicalState)
	require.Equal(t, "40 Gb/sec (4X QDR)", parsed[8].Rate)
	require.Equal(t, "Ethernet", parsed[8].LinkLayer)
}

func TestParseIBStatusEmptyInput(t *testing.T) {
	t.Parallel()

	parsed, err := ParseIBStatus("")
	require.Error(t, err)
	require.Equal(t, ErrIbstatusOutputEmpty, err)
	require.Nil(t, parsed)
}

func TestParseIBStatusInvalidInput(t *testing.T) {
	t.Parallel()

	invalidInput := "This is not a valid ibstatus output"
	parsed, err := ParseIBStatus(invalidInput)
	require.Error(t, err)
	require.Nil(t, parsed)
}

func TestParseIBStatusNoDeviceFound(t *testing.T) {
	t.Parallel()

	// Create an empty YAML object
	noDeviceInput := `{}`
	parsed, err := ParseIBStatus(noDeviceInput)
	require.Error(t, err)
	require.Equal(t, ErrIbstatusOutputNoDeviceFound, err)
	require.Nil(t, parsed)
}

func TestGetIbstatusOutputNoCommand(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	output, err := GetIbstatusOutput(ctx, []string{})
	require.Error(t, err)
	require.Equal(t, ErrNoIbstatusCommand, err)
	require.Nil(t, output)

	output, err = GetIbstatusOutput(ctx, []string{""})
	require.Error(t, err)
	require.Equal(t, ErrNoIbstatusCommand, err)
	require.Nil(t, output)
}

// TestParseIBStatusMixedStates tests parsing output with both active and down states
func TestParseIBStatusMixedStates(t *testing.T) {
	t.Parallel()

	mixedStatesInput := `Infiniband device 'mlx5_0' port 1 status:
        default gid:     fe80:0000:0000:0000:0015:5dff:fd34:11eb
        base lid:        0x33bf
        sm lid:          0x1
        state:           4: ACTIVE
        phys state:      5: LinkUp
        rate:            200 Gb/sec (4X HDR)
        link_layer:      InfiniBand

Infiniband device 'mlx5_1' port 1 status:
        default gid:     fe80:0000:0000:0000:0015:5dff:fd34:11ec
        base lid:        0x0
        sm lid:          0x0
        state:           1: DOWN
        phys state:      2: Disabled
        rate:            200 Gb/sec (4X HDR)
        link_layer:      InfiniBand`

	parsed, err := ParseIBStatus(mixedStatesInput)
	require.NoError(t, err)
	require.NotEmpty(t, parsed)
	require.Equal(t, 2, len(parsed))

	// Check first device (active)
	require.Equal(t, "mlx5_0", parsed[0].Device)
	require.Equal(t, "4: ACTIVE", parsed[0].State)
	require.Equal(t, "5: LinkUp", parsed[0].PhysicalState)

	// Check second device (down)
	require.Equal(t, "mlx5_1", parsed[1].Device)
	require.Equal(t, "1: DOWN", parsed[1].State)
	require.Equal(t, "2: Disabled", parsed[1].PhysicalState)
}

// TestParseIBStatusIncompleteFields tests parsing output with some missing fields
func TestParseIBStatusIncompleteFields(t *testing.T) {
	t.Parallel()

	incompleteInput := `Infiniband device 'mlx5_0' port 1 status:
        default gid:     fe80:0000:0000:0000:0015:5dff:fd34:11eb
        state:           4: ACTIVE
        phys state:      5: LinkUp
        link_layer:      InfiniBand`

	parsed, err := ParseIBStatus(incompleteInput)
	require.NoError(t, err)
	require.NotEmpty(t, parsed)
	require.Equal(t, 1, len(parsed))

	// Check that parsed fields are correct and missing fields are empty
	require.Equal(t, "mlx5_0", parsed[0].Device)
	require.Equal(t, "4: ACTIVE", parsed[0].State)
	require.Equal(t, "5: LinkUp", parsed[0].PhysicalState)
	require.Equal(t, "InfiniBand", parsed[0].LinkLayer)
	require.Equal(t, "fe80:0000:0000:0000:0015:5dff:fd34:11eb", parsed[0].DefaultGID)
	require.Equal(t, "", parsed[0].DefaultLID)
	require.Equal(t, "", parsed[0].SMLID)
	require.Equal(t, "", parsed[0].Rate)
	require.Equal(t, "", parsed[0].BaseLid)
}

// TestGetIbstatusOutputWithMockedCommand tests GetIbstatusOutput with a mocked command
func TestGetIbstatusOutputWithMockedCommand(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// Create a temporary shell script that outputs valid ibstatus format
	tmpScript := `#!/bin/bash
echo 'Infiniband device '"'"'mlx5_0'"'"' port 1 status:
        default gid:     fe80:0000:0000:0000:0015:5dff:fd34:11eb
        base lid:        0x33bf
        sm lid:          0x1
        state:           4: ACTIVE
        phys state:      5: LinkUp
        rate:            200 Gb/sec (4X HDR)
        link_layer:      InfiniBand'
`
	tmpFile := "/tmp/mock_ibstatus.sh"
	err := os.WriteFile(tmpFile, []byte(tmpScript), 0755)
	require.NoError(t, err)
	defer os.Remove(tmpFile)

	output, err := GetIbstatusOutput(ctx, []string{tmpFile})
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotEmpty(t, output.Raw)
	require.NotEmpty(t, output.Parsed)
	require.Equal(t, 1, len(output.Parsed))
	require.Equal(t, "mlx5_0", output.Parsed[0].Device)
}

// TestGetIbstatusOutputWithNonExistentCommand tests behavior when command doesn't exist
func TestGetIbstatusOutputWithNonExistentCommand(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	output, err := GetIbstatusOutput(ctx, []string{"non_existent_command"})
	require.Error(t, err)
	require.Equal(t, ErrNoIbstatusCommand, err)
	require.Nil(t, output)
}

// TestGetIbstatusOutputWithFailingCommand tests behavior when command fails but returns output
func TestGetIbstatusOutputWithFailingCommand(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// Create a temporary script that outputs error data and exits with error
	tmpScript := `#!/bin/bash
echo 'Error output'
exit 1
`
	tmpFile := "/tmp/failing_ibstatus.sh"
	err := os.WriteFile(tmpFile, []byte(tmpScript), 0755)
	require.NoError(t, err)
	defer os.Remove(tmpFile)

	output, err := GetIbstatusOutput(ctx, []string{tmpFile})
	require.Error(t, err)
	require.NotNil(t, output)
	require.Equal(t, "Error output", output.Raw)
}

// TestParseIBStatusWithSpecialCharacters tests parsing output with special characters
func TestParseIBStatusWithSpecialCharacters(t *testing.T) {
	t.Parallel()

	specialCharsInput := `Infiniband device 'mlx5_0' port 1 status:
        default gid:     fe80:0000:0000:0000:0015:5dff:fd34:11eb
        base lid:        0x33bf
        sm lid:          0x1
        state:           4: ACTIVE (with special characters)
        phys state:      5: LinkUp [status]
        rate:            200 Gb/sec (4X HDR) high speed
        link_layer:      InfiniBand`

	parsed, err := ParseIBStatus(specialCharsInput)
	require.NoError(t, err)
	require.NotEmpty(t, parsed)
	require.Equal(t, 1, len(parsed))
	require.Equal(t, "mlx5_0", parsed[0].Device)
	require.Equal(t, "4: ACTIVE (with special characters)", parsed[0].State)
	require.Equal(t, "5: LinkUp [status]", parsed[0].PhysicalState)
	require.Equal(t, "200 Gb/sec (4X HDR) high speed", parsed[0].Rate)
}
