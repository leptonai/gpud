package infiniband

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	infinibandclass "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/class"
)

func TestComponentReadClass(t *testing.T) {
	origClassDir := "../../../../pkg/nvidia-query/infiniband/class/testdata/sys-class-infiniband-h100.0"
	if _, err := os.Stat(origClassDir); err != nil {
		t.Skip("skipping test, test class dir does not exist")
	}

	classRootDir := copyTestClassDir(t, origClassDir)
	defer os.RemoveAll(classRootDir)

	es := &mockEventStore{}
	bucket, _ := es.Bucket(Name)

	timeNow := time.Now().UTC()
	threshold := infiniband.ExpectedPortStates{}
	c := &component{
		ctx:    context.Background(),
		cancel: func() {},

		checkInterval:  time.Minute,
		requestTimeout: 15 * time.Second,

		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},

		eventBucket: bucket,

		getTimeNowFunc: func() time.Time {
			return timeNow
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return threshold
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.LoadDevices(classRootDir)
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			// Mock function to prevent nil pointer dereference
			return nil, infiniband.ErrNoIbstatCommand
		},
	}

	// Test case 1: Basic healthy scenario with sufficient ports and rate
	timeNow = timeNow.Add(time.Minute)
	threshold = infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
	cr := c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	assert.Equal(t, cr.Summary(), "ok; no infiniband port issue")

	// Test case 2: Unhealthy scenario - require more ports than available
	timeNow = timeNow.Add(time.Minute)
	threshold = infiniband.ExpectedPortStates{AtLeastPorts: 10, AtLeastRate: 400}
	cr = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	assert.Equal(t, cr.Summary(), "only 8 port(s) are active and >=400 Gb/s, expect >=10 port(s)")

	// Test case 3: Test with port state changes - set some ports to Down
	updatePortState(t, classRootDir, "mlx5_0", 1, "Down")
	updatePortState(t, classRootDir, "mlx5_1", 1, "Down")

	timeNow = timeNow.Add(time.Minute)
	threshold = infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
	cr = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType()) // Should be healthy with 6 ports active
	assert.Equal(t, cr.Summary(), "ok; no infiniband port issue")

	// Test case 4: Test with physical state changes - this should make it unhealthy
	updatePortPhysState(t, classRootDir, "mlx5_4", 1, "Disabled")
	updatePortPhysState(t, classRootDir, "mlx5_5", 1, "Disabled")

	timeNow = timeNow.Add(time.Minute)
	threshold = infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
	cr = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	assert.Contains(t, cr.Summary(), "only 6 port(s) are active and >=400 Gb/s, expect >=8 port(s)")

	// Test case 5: Test with rate changes - reduce rate below threshold
	updatePortRate(t, classRootDir, "mlx5_6", 1, "200 Gb/sec (2X EDR)")
	updatePortRate(t, classRootDir, "mlx5_7", 1, "200 Gb/sec (2X EDR)")

	timeNow = timeNow.Add(time.Minute)
	threshold = infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
	cr = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	assert.Contains(t, cr.Summary(), "only 4 port(s) are active and >=400 Gb/s, expect >=8 port(s)")

	// Test case 6: Test with link_downed counter increases
	updateLinkDownedCounter(t, classRootDir, "mlx5_8", 1, 5)
	updateLinkDownedCounter(t, classRootDir, "mlx5_9", 1, 3)

	timeNow = timeNow.Add(time.Minute)
	threshold = infiniband.ExpectedPortStates{AtLeastPorts: 2, AtLeastRate: 400}
	cr = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	assert.Equal(t, cr.Summary(), "ok; no infiniband port issue")

	// Test case 7: Test with various error counters
	updateErrorCounter(t, classRootDir, "mlx5_8", 1, "symbol_error", 10)
	updateErrorCounter(t, classRootDir, "mlx5_8", 1, "port_rcv_errors", 25)
	updateErrorCounter(t, classRootDir, "mlx5_9", 1, "local_link_integrity_errors", 15)
	updateErrorCounter(t, classRootDir, "mlx5_9", 1, "excessive_buffer_overrun_errors", 7)

	timeNow = timeNow.Add(time.Minute)
	threshold = infiniband.ExpectedPortStates{AtLeastPorts: 2, AtLeastRate: 400}
	cr = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	assert.Equal(t, cr.Summary(), "ok; no infiniband port issue")

	// Test case 8: Restore some ports to test recovery scenario
	updatePortState(t, classRootDir, "mlx5_0", 1, "Active")
	updatePortState(t, classRootDir, "mlx5_1", 1, "Active")
	updatePortPhysState(t, classRootDir, "mlx5_0", 1, "LinkUp")
	updatePortPhysState(t, classRootDir, "mlx5_1", 1, "LinkUp")
	updatePortRate(t, classRootDir, "mlx5_0", 1, "400 Gb/sec (4X EDR)")
	updatePortRate(t, classRootDir, "mlx5_1", 1, "400 Gb/sec (4X EDR)")

	timeNow = timeNow.Add(time.Minute)
	threshold = infiniband.ExpectedPortStates{AtLeastPorts: 4, AtLeastRate: 400}
	cr = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	assert.Equal(t, cr.Summary(), "ok; no infiniband port issue")

	// Test case 9: Test edge case - exactly meeting threshold
	timeNow = timeNow.Add(time.Minute)
	threshold = infiniband.ExpectedPortStates{AtLeastPorts: 4, AtLeastRate: 400}
	cr = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	assert.Equal(t, cr.Summary(), "ok; no infiniband port issue")

	// Test case 10: Test with all main ports down
	updatePortState(t, classRootDir, "mlx5_0", 1, "Down")
	updatePortState(t, classRootDir, "mlx5_1", 1, "Down")
	updatePortState(t, classRootDir, "mlx5_8", 1, "Down")
	updatePortState(t, classRootDir, "mlx5_9", 1, "Down")

	timeNow = timeNow.Add(time.Minute)
	threshold = infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 400}
	cr = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType()) // Still healthy because other ports are active
	assert.Equal(t, cr.Summary(), "ok; no infiniband port issue")

	// Test case 11: Test with mixed states and rates
	updatePortState(t, classRootDir, "mlx5_0", 1, "Active")
	updatePortPhysState(t, classRootDir, "mlx5_0", 1, "LinkUp")
	updatePortRate(t, classRootDir, "mlx5_0", 1, "200 Gb/sec (2X EDR)")
	updatePortState(t, classRootDir, "mlx5_1", 1, "Active")
	updatePortPhysState(t, classRootDir, "mlx5_1", 1, "LinkUp")
	updatePortRate(t, classRootDir, "mlx5_1", 1, "400 Gb/sec (4X EDR)")

	timeNow = timeNow.Add(time.Minute)
	threshold = infiniband.ExpectedPortStates{AtLeastPorts: 2, AtLeastRate: 400}
	cr = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType()) // Still healthy because other ports meet threshold
	assert.Equal(t, cr.Summary(), "ok; no infiniband port issue")

	// Test case 12: Test high error counter scenario
	updateErrorCounter(t, classRootDir, "mlx5_0", 1, "symbol_error", 1000)
	updateErrorCounter(t, classRootDir, "mlx5_0", 1, "port_rcv_errors", 500)
	updateErrorCounter(t, classRootDir, "mlx5_1", 1, "local_link_integrity_errors", 750)
	updateLinkDownedCounter(t, classRootDir, "mlx5_0", 1, 50)
	updateLinkDownedCounter(t, classRootDir, "mlx5_1", 1, 25)

	timeNow = timeNow.Add(time.Minute)
	threshold = infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 200}
	cr = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	assert.Equal(t, cr.Summary(), "ok; no infiniband port issue")
}

// Helper function to update port state file
func updatePortState(t *testing.T, classRootDir, device string, port int, state string) {
	stateFile := filepath.Join(classRootDir, device, "ports", fmt.Sprintf("%d", port), "state")
	var stateContent string
	switch state {
	case "Active":
		stateContent = "4: ACTIVE"
	case "Down":
		stateContent = "2: DOWN"
	case "Init":
		stateContent = "1: INIT"
	case "Armed":
		stateContent = "3: ARMED"
	default:
		stateContent = "4: ACTIVE" // default to active
	}
	assert.NoError(t, os.WriteFile(stateFile, []byte(stateContent), 0644))
}

// Helper function to update port physical state file
func updatePortPhysState(t *testing.T, classRootDir, device string, port int, physState string) {
	physStateFile := filepath.Join(classRootDir, device, "ports", fmt.Sprintf("%d", port), "phys_state")
	var physStateContent string
	switch physState {
	case "LinkUp":
		physStateContent = "5: LinkUp"
	case "Disabled":
		physStateContent = "3: Disabled"
	case "Polling":
		physStateContent = "2: Polling"
	case "Sleep":
		physStateContent = "1: Sleep"
	default:
		physStateContent = "5: LinkUp" // default to LinkUp
	}
	assert.NoError(t, os.WriteFile(physStateFile, []byte(physStateContent), 0644))
}

// Helper function to update link_downed counter
func updateLinkDownedCounter(t *testing.T, classRootDir, device string, port int, count uint64) {
	counterFile := filepath.Join(classRootDir, device, "ports", fmt.Sprintf("%d", port), "counters", "link_downed")
	assert.NoError(t, os.WriteFile(counterFile, []byte(fmt.Sprintf("%d", count)), 0644))
}

// Helper function to update port rate
func updatePortRate(t *testing.T, classRootDir, device string, port int, rate string) {
	rateFile := filepath.Join(classRootDir, device, "ports", fmt.Sprintf("%d", port), "rate")
	assert.NoError(t, os.WriteFile(rateFile, []byte(rate), 0644))
}

// Helper function to update error counters
func updateErrorCounter(t *testing.T, classRootDir, device string, port int, counterName string, value uint64) {
	counterFile := filepath.Join(classRootDir, device, "ports", fmt.Sprintf("%d", port), "counters", counterName)
	assert.NoError(t, os.WriteFile(counterFile, []byte(fmt.Sprintf("%d", value)), 0644))
}

// Helper function to copy test class directory
func copyTestClassDir(t *testing.T, origClassDir string) string {
	classRootDir, err := os.MkdirTemp(t.TempDir(), "gpud-test-class-dir")
	assert.NoError(t, err)

	// recursively copy "testClassDir" directory to "tmpDir"
	assert.NoError(t, filepath.WalkDir(origClassDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(origClassDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(classRootDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = destFile.ReadFrom(srcFile)
		return err
	}))

	return classRootDir
}
