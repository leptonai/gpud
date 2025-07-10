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
	infinibandstore "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/store"
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

// TestComponentReadClass_FlapDetection tests InfiniBand port flapping detection scenarios
func TestComponentReadClass_FlapDetection(t *testing.T) {
	origClassDir := "../../../../pkg/nvidia-query/infiniband/class/testdata/sys-class-infiniband-h100.0"
	if _, err := os.Stat(origClassDir); err != nil {
		t.Skip("skipping test, test class dir does not exist")
	}

	classRootDir := copyTestClassDir(t, origClassDir)
	defer os.RemoveAll(classRootDir)

	es := &mockEventStore{}
	bucket, _ := es.Bucket(Name)

	baseTime := time.Now().UTC()

	// Test case 1: Classic Port Flap - Port bounces between Active and Down
	t.Run("classic_port_flap", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime
		linkDownedCount := uint64(10)

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
		}

		// Initial state - all ports active
		cr := c.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())

		// Simulate flap sequence 1: Port goes down
		updatePortState(t, classRootDir, "mlx5_0", 1, "Down")
		linkDownedCount++
		updateLinkDownedCounter(t, classRootDir, "mlx5_0", 1, linkDownedCount)

		timeNow = timeNow.Add(30 * time.Second)
		cr = c.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType()) // Still have 7 active ports

		// Port comes back up
		updatePortState(t, classRootDir, "mlx5_0", 1, "Active")
		timeNow = timeNow.Add(30 * time.Second)
		cr = c.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())

		// Simulate flap sequence 2: Another port flaps
		updatePortState(t, classRootDir, "mlx5_1", 1, "Down")
		linkDownedCount++
		updateLinkDownedCounter(t, classRootDir, "mlx5_1", 1, linkDownedCount)

		timeNow = timeNow.Add(30 * time.Second)
		cr = c.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())

		// Port comes back up again
		updatePortState(t, classRootDir, "mlx5_1", 1, "Active")
		timeNow = timeNow.Add(30 * time.Second)
		cr = c.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})

	// Test case 2: Rapid Flapping - Multiple quick state changes
	t.Run("rapid_flapping", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime.Add(10 * time.Minute)
		linkDownedCount := uint64(20)

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
		}

		// Simulate rapid flapping on multiple ports
		for i := 0; i < 5; i++ {
			// Ports go down
			updatePortState(t, classRootDir, "mlx5_0", 1, "Down")
			updatePortState(t, classRootDir, "mlx5_1", 1, "Down")
			linkDownedCount += 2
			updateLinkDownedCounter(t, classRootDir, "mlx5_0", 1, linkDownedCount)
			updateLinkDownedCounter(t, classRootDir, "mlx5_1", 1, linkDownedCount+1)

			timeNow = timeNow.Add(10 * time.Second)
			cr := c.Check()
			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType()) // Still have 6 active ports

			// Ports come back up
			updatePortState(t, classRootDir, "mlx5_0", 1, "Active")
			updatePortState(t, classRootDir, "mlx5_1", 1, "Active")

			timeNow = timeNow.Add(10 * time.Second)
			cr = c.Check()
			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
		}
	})

	// Test case 3: Flap with increasing error counters
	t.Run("flap_with_error_counters", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime.Add(20 * time.Minute)
		linkDownedCount := uint64(30)
		errorCount := uint64(100)

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
		}

		// Port flaps with increasing error counters
		for i := 0; i < 3; i++ {
			// Port goes down with errors
			updatePortState(t, classRootDir, "mlx5_4", 1, "Down")
			linkDownedCount++
			updateLinkDownedCounter(t, classRootDir, "mlx5_4", 1, linkDownedCount)

			// Increase error counters
			errorCount += 50
			updateErrorCounter(t, classRootDir, "mlx5_4", 1, "port_rcv_errors", errorCount)
			updateErrorCounter(t, classRootDir, "mlx5_4", 1, "symbol_error", errorCount/2)

			timeNow = timeNow.Add(30 * time.Second)
			cr := c.Check()
			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())

			// Port comes back up
			updatePortState(t, classRootDir, "mlx5_4", 1, "Active")
			timeNow = timeNow.Add(30 * time.Second)
			cr = c.Check()
			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
		}
	})
}

// TestComponentReadClass_DropDetection tests InfiniBand port drop detection scenarios
func TestComponentReadClass_DropDetection(t *testing.T) {
	origClassDir := "../../../../pkg/nvidia-query/infiniband/class/testdata/sys-class-infiniband-h100.0"
	if _, err := os.Stat(origClassDir); err != nil {
		t.Skip("skipping test, test class dir does not exist")
	}

	classRootDir := copyTestClassDir(t, origClassDir)
	defer os.RemoveAll(classRootDir)

	es := &mockEventStore{}
	bucket, _ := es.Bucket(Name)

	baseTime := time.Now().UTC()

	// Test case 1: Persistent Port Drop - Port stays down for extended period
	t.Run("persistent_port_drop", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime
		linkDownedCount := uint64(10)

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
		}

		// Set multiple ports down to trigger unhealthy state
		updatePortState(t, classRootDir, "mlx5_0", 1, "Down")
		updatePortState(t, classRootDir, "mlx5_1", 1, "Down")
		updatePortState(t, classRootDir, "mlx5_4", 1, "Down")
		updatePortPhysState(t, classRootDir, "mlx5_0", 1, "Disabled")
		updatePortPhysState(t, classRootDir, "mlx5_1", 1, "Disabled")
		updatePortPhysState(t, classRootDir, "mlx5_4", 1, "Disabled")
		updateLinkDownedCounter(t, classRootDir, "mlx5_0", 1, linkDownedCount)
		updateLinkDownedCounter(t, classRootDir, "mlx5_1", 1, linkDownedCount)
		updateLinkDownedCounter(t, classRootDir, "mlx5_4", 1, linkDownedCount)

		// Check multiple times over 5 minutes without link_downed counter changing
		for i := 0; i < 10; i++ {
			timeNow = timeNow.Add(30 * time.Second)
			cr := c.Check()
			assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType()) // 5 active ports, need 8, so unhealthy
			assert.Contains(t, cr.Summary(), "only 5 port(s) are active")
		}
	})

	// Test case 2: Drop with High Error Rate
	t.Run("drop_with_high_error_rate", func(t *testing.T) {
		// Reset all ports to healthy state first
		for _, device := range []string{"mlx5_0", "mlx5_1", "mlx5_4", "mlx5_5", "mlx5_6", "mlx5_7", "mlx5_8", "mlx5_9"} {
			updatePortState(t, classRootDir, device, 1, "Active")
			updatePortPhysState(t, classRootDir, device, 1, "LinkUp")
		}

		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime.Add(10 * time.Minute)
		errorCount := uint64(1000)

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
		}

		// Simulate rapidly increasing error counters (packet drops)
		for i := 0; i < 5; i++ {
			errorCount += 500
			updateErrorCounter(t, classRootDir, "mlx5_5", 1, "port_rcv_errors", errorCount)
			updateErrorCounter(t, classRootDir, "mlx5_5", 1, "port_xmit_discards", errorCount/2)
			updateErrorCounter(t, classRootDir, "mlx5_5", 1, "symbol_error", errorCount/4)

			timeNow = timeNow.Add(30 * time.Second)
			cr := c.Check()
			// Still healthy because ports are active, but error counters are increasing
			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
		}
	})

	// Test case 3: Drop Recovery - Port recovers after long down period
	t.Run("drop_recovery", func(t *testing.T) {
		// Reset all ports to healthy state first
		for _, device := range []string{"mlx5_0", "mlx5_1", "mlx5_4", "mlx5_5", "mlx5_6", "mlx5_7", "mlx5_8", "mlx5_9"} {
			updatePortState(t, classRootDir, device, 1, "Active")
			updatePortPhysState(t, classRootDir, device, 1, "LinkUp")
		}

		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime.Add(20 * time.Minute)
		linkDownedCount := uint64(15)

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
		}

		// Ports go down
		updatePortState(t, classRootDir, "mlx5_6", 1, "Down")
		updatePortState(t, classRootDir, "mlx5_7", 1, "Down")
		updatePortState(t, classRootDir, "mlx5_8", 1, "Down")
		updatePortPhysState(t, classRootDir, "mlx5_6", 1, "Disabled")
		updatePortPhysState(t, classRootDir, "mlx5_7", 1, "Disabled")
		updatePortPhysState(t, classRootDir, "mlx5_8", 1, "Disabled")
		updateLinkDownedCounter(t, classRootDir, "mlx5_6", 1, linkDownedCount)
		updateLinkDownedCounter(t, classRootDir, "mlx5_7", 1, linkDownedCount)
		updateLinkDownedCounter(t, classRootDir, "mlx5_8", 1, linkDownedCount)

		// Stay down for 5 minutes
		for i := 0; i < 10; i++ {
			timeNow = timeNow.Add(30 * time.Second)
			cr := c.Check()
			assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
		}

		// Ports recover
		updatePortState(t, classRootDir, "mlx5_6", 1, "Active")
		updatePortState(t, classRootDir, "mlx5_7", 1, "Active")
		updatePortState(t, classRootDir, "mlx5_8", 1, "Active")
		updatePortPhysState(t, classRootDir, "mlx5_6", 1, "LinkUp")
		updatePortPhysState(t, classRootDir, "mlx5_7", 1, "LinkUp")
		updatePortPhysState(t, classRootDir, "mlx5_8", 1, "LinkUp")

		timeNow = timeNow.Add(30 * time.Second)
		cr := c.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
		assert.Equal(t, "ok; no infiniband port issue", cr.Summary())
	})
}

// TestComponentReadClass_CombinedFlapAndDrop tests combined flap and drop scenarios
func TestComponentReadClass_CombinedFlapAndDrop(t *testing.T) {
	origClassDir := "../../../../pkg/nvidia-query/infiniband/class/testdata/sys-class-infiniband-h100.0"
	if _, err := os.Stat(origClassDir); err != nil {
		t.Skip("skipping test, test class dir does not exist")
	}

	classRootDir := copyTestClassDir(t, origClassDir)
	defer os.RemoveAll(classRootDir)

	es := &mockEventStore{}
	bucket, _ := es.Bucket(Name)

	baseTime := time.Now().UTC()

	// Test case: Some ports flapping while others are persistently down
	t.Run("mixed_flap_and_drop", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 6, AtLeastRate: 400}
		timeNow := baseTime
		linkDownedCount := uint64(20)

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
		}

		// Set some ports persistently down (drop scenario)
		updatePortState(t, classRootDir, "mlx5_0", 1, "Down")
		updatePortState(t, classRootDir, "mlx5_1", 1, "Down")
		updatePortPhysState(t, classRootDir, "mlx5_0", 1, "Disabled")
		updatePortPhysState(t, classRootDir, "mlx5_1", 1, "Disabled")
		updateLinkDownedCounter(t, classRootDir, "mlx5_0", 1, linkDownedCount)
		updateLinkDownedCounter(t, classRootDir, "mlx5_1", 1, linkDownedCount)

		// Other ports will flap
		for i := 0; i < 3; i++ {
			// mlx5_4 and mlx5_5 flap
			updatePortState(t, classRootDir, "mlx5_4", 1, "Down")
			updatePortState(t, classRootDir, "mlx5_5", 1, "Down")
			updatePortPhysState(t, classRootDir, "mlx5_4", 1, "Disabled")
			updatePortPhysState(t, classRootDir, "mlx5_5", 1, "Disabled")
			linkDownedCount++
			updateLinkDownedCounter(t, classRootDir, "mlx5_4", 1, linkDownedCount)
			updateLinkDownedCounter(t, classRootDir, "mlx5_5", 1, linkDownedCount+1)

			timeNow = timeNow.Add(30 * time.Second)
			cr := c.Check()
			assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
			assert.Contains(t, cr.Summary(), "only 4 port(s) are active")

			// Flapping ports come back up
			updatePortState(t, classRootDir, "mlx5_4", 1, "Active")
			updatePortState(t, classRootDir, "mlx5_5", 1, "Active")
			updatePortPhysState(t, classRootDir, "mlx5_4", 1, "LinkUp")
			updatePortPhysState(t, classRootDir, "mlx5_5", 1, "LinkUp")

			timeNow = timeNow.Add(30 * time.Second)
			cr = c.Check()
			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType()) // Back to 6 active ports

			// Add increasing error counters during flaps
			updateErrorCounter(t, classRootDir, "mlx5_4", 1, "port_rcv_errors", uint64(100*(i+1)))
			updateErrorCounter(t, classRootDir, "mlx5_5", 1, "symbol_error", uint64(50*(i+1)))
		}

		// mlx5_0 and mlx5_1 remain down throughout (persistent drop)
		timeNow = timeNow.Add(3 * time.Minute)
		cr := c.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})
}

// TestComponentReadClass_ErrorCounterRates tests detection of high-rate packet drops through error counters
func TestComponentReadClass_ErrorCounterRates(t *testing.T) {
	origClassDir := "../../../../pkg/nvidia-query/infiniband/class/testdata/sys-class-infiniband-h100.0"
	if _, err := os.Stat(origClassDir); err != nil {
		t.Skip("skipping test, test class dir does not exist")
	}

	classRootDir := copyTestClassDir(t, origClassDir)
	defer os.RemoveAll(classRootDir)

	es := &mockEventStore{}
	bucket, _ := es.Bucket(Name)

	baseTime := time.Now().UTC()

	// Test case 1: High rate of receive errors
	t.Run("high_rate_receive_errors", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime

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
		}

		// Simulate rapid increase in receive errors (1000 errors per 30 seconds)
		baseErrors := uint64(100)
		for i := 0; i < 5; i++ {
			baseErrors += 1000
			updateErrorCounter(t, classRootDir, "mlx5_0", 1, "port_rcv_errors", baseErrors)

			timeNow = timeNow.Add(30 * time.Second)
			cr := c.Check()
			// Component currently only checks port states, not error rates
			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
		}
	})

	// Test case 2: Multiple error types increasing simultaneously
	t.Run("multiple_error_types", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime.Add(10 * time.Minute)

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
		}

		// Multiple error counters increasing
		rcvErrors := uint64(100)
		xmitDiscards := uint64(50)
		symbolErrors := uint64(25)
		linkIntegrityErrors := uint64(10)

		for i := 0; i < 3; i++ {
			rcvErrors += 500
			xmitDiscards += 250
			symbolErrors += 125
			linkIntegrityErrors += 50

			updateErrorCounter(t, classRootDir, "mlx5_1", 1, "port_rcv_errors", rcvErrors)
			updateErrorCounter(t, classRootDir, "mlx5_1", 1, "port_xmit_discards", xmitDiscards)
			updateErrorCounter(t, classRootDir, "mlx5_1", 1, "symbol_error", symbolErrors)
			updateErrorCounter(t, classRootDir, "mlx5_1", 1, "local_link_integrity_errors", linkIntegrityErrors)

			timeNow = timeNow.Add(30 * time.Second)
			cr := c.Check()
			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
		}
	})

	// Test case 3: Error burst followed by quiet period
	t.Run("error_burst_pattern", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime.Add(20 * time.Minute)

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
		}

		// Burst of errors
		errors := uint64(100)
		for i := 0; i < 3; i++ {
			errors += 5000 // Large burst
			updateErrorCounter(t, classRootDir, "mlx5_6", 1, "port_rcv_errors", errors)

			timeNow = timeNow.Add(10 * time.Second)
			cr := c.Check()
			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
		}

		// Quiet period - no new errors for 5 minutes
		for i := 0; i < 10; i++ {
			timeNow = timeNow.Add(30 * time.Second)
			cr := c.Check()
			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
		}
	})
}

// TestComponentReadClass_EdgeCases tests edge cases and boundary conditions
func TestComponentReadClass_EdgeCases(t *testing.T) {
	origClassDir := "../../../../pkg/nvidia-query/infiniband/class/testdata/sys-class-infiniband-h100.0"
	if _, err := os.Stat(origClassDir); err != nil {
		t.Skip("skipping test, test class dir does not exist")
	}

	classRootDir := copyTestClassDir(t, origClassDir)
	defer os.RemoveAll(classRootDir)

	es := &mockEventStore{}
	bucket, _ := es.Bucket(Name)

	baseTime := time.Now().UTC()

	// Test case 1: Counter reset scenario
	t.Run("counter_reset", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime

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
		}

		// Set high counter values
		updateLinkDownedCounter(t, classRootDir, "mlx5_0", 1, 1000)
		updateErrorCounter(t, classRootDir, "mlx5_0", 1, "port_rcv_errors", 50000)

		timeNow = timeNow.Add(30 * time.Second)
		cr := c.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())

		// Counter reset (goes back to zero or low value)
		updateLinkDownedCounter(t, classRootDir, "mlx5_0", 1, 0)
		updateErrorCounter(t, classRootDir, "mlx5_0", 1, "port_rcv_errors", 0)

		timeNow = timeNow.Add(30 * time.Second)
		cr = c.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})

	// Test case 2: All ports flapping simultaneously
	t.Run("all_ports_flapping", func(t *testing.T) {
		// Create a fresh copy for this subtest
		subClassRootDir := copyTestClassDir(t, origClassDir)
		defer os.RemoveAll(subClassRootDir)
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime.Add(10 * time.Minute)
		linkDownedBase := uint64(100)

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
				return infinibandclass.LoadDevices(subClassRootDir)
			},
		}

		// All InfiniBand ports go down simultaneously (excluding Ethernet bond)
		// Only InfiniBand ports are counted by the component due to IsIBPort() filtering
		ibDevices := []string{"mlx5_0", "mlx5_1", "mlx5_4", "mlx5_5", "mlx5_6", "mlx5_7", "mlx5_8", "mlx5_9"}
		for i, device := range ibDevices {
			updatePortState(t, subClassRootDir, device, 1, "Down")
			updatePortPhysState(t, subClassRootDir, device, 1, "Disabled")
			updateLinkDownedCounter(t, subClassRootDir, device, 1, linkDownedBase+uint64(i))
		}

		timeNow = timeNow.Add(30 * time.Second)
		cr := c.Check()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
		assert.Contains(t, cr.Summary(), "only 0 port(s) are active")

		// All InfiniBand ports come back up
		for _, device := range ibDevices {
			updatePortState(t, subClassRootDir, device, 1, "Active")
			updatePortPhysState(t, subClassRootDir, device, 1, "LinkUp")
		}

		timeNow = timeNow.Add(30 * time.Second)
		cr = c.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})

	// Test case 3: Rate degradation without port down
	t.Run("rate_degradation", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime.Add(20 * time.Minute)

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
		}

		// Degrade rate on multiple ports while keeping them active
		updatePortRate(t, classRootDir, "mlx5_0", 1, "200 Gb/sec (2X EDR)")
		updatePortRate(t, classRootDir, "mlx5_1", 1, "200 Gb/sec (2X EDR)")
		updatePortRate(t, classRootDir, "mlx5_4", 1, "100 Gb/sec (1X EDR)")

		timeNow = timeNow.Add(30 * time.Second)
		cr := c.Check()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
		assert.Contains(t, cr.Summary(), "only 5 port(s) are active and >=400 Gb/s")

		// Restore rates
		updatePortRate(t, classRootDir, "mlx5_0", 1, "400 Gb/sec (4X EDR)")
		updatePortRate(t, classRootDir, "mlx5_1", 1, "400 Gb/sec (4X EDR)")
		updatePortRate(t, classRootDir, "mlx5_4", 1, "400 Gb/sec (4X EDR)")

		timeNow = timeNow.Add(30 * time.Second)
		cr = c.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})

	// Test case 4: Physical state changes without logical state changes
	t.Run("physical_state_changes", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime.Add(30 * time.Minute)

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
		}

		// Change physical state while keeping logical state active
		updatePortPhysState(t, classRootDir, "mlx5_0", 1, "Polling")
		updatePortPhysState(t, classRootDir, "mlx5_1", 1, "Sleep")

		timeNow = timeNow.Add(30 * time.Second)
		cr := c.Check()
		// Physical state changes affect health - 2 ports now have non-LinkUp phys state
		// This reduces active ports from 8 to 6 (below the 8 port threshold)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())

		// Disable ports physically
		updatePortPhysState(t, classRootDir, "mlx5_0", 1, "Disabled")
		updatePortPhysState(t, classRootDir, "mlx5_1", 1, "Disabled")
		updatePortPhysState(t, classRootDir, "mlx5_4", 1, "Disabled")

		timeNow = timeNow.Add(30 * time.Second)
		cr = c.Check()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	})
}

// Mock implementations for testing

// mockIBPortsStore implements infinibandstore.Store interface for testing
type mockIBPortsStore struct {
	events []infinibandstore.Event
}

func (m *mockIBPortsStore) Insert(eventTime time.Time, ibPorts []infiniband.IBPort) error {
	return nil
}

func (m *mockIBPortsStore) SetEventType(device string, port uint, timestamp time.Time, eventType string, eventReason string) error {
	return nil
}

func (m *mockIBPortsStore) LastEvents(since time.Time) ([]infinibandstore.Event, error) {
	return m.events, nil
}

func (m *mockIBPortsStore) Tombstone(timestamp time.Time) error {
	return nil
}

func (m *mockIBPortsStore) Scan() error {
	return nil
}

// TestComponentReadClass_EventReporting tests the event reporting format logic
func TestComponentReadClass_EventReporting(t *testing.T) {
	origClassDir := "../../../../pkg/nvidia-query/infiniband/class/testdata/sys-class-infiniband-h100.0"
	if _, err := os.Stat(origClassDir); err != nil {
		t.Skip("skipping test, test class dir does not exist")
	}

	classRootDir := copyTestClassDir(t, origClassDir)
	defer os.RemoveAll(classRootDir)

	es := &mockEventStore{}
	bucket, _ := es.Bucket(Name)

	baseTime := time.Now().UTC()

	// Test the event reporting format: device/port with proper EventReason
	t.Run("event_format_verification", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime

		// Create a mock store that will simulate flap/drop events with proper EventReason
		mockStore := &mockIBPortsStore{
			events: []infinibandstore.Event{
				{
					Time: timeNow,
					Port: infiniband.IBPort{
						Device: "mlx5_0",
						Port:   1,
					},
					EventType:   infinibandstore.EventTypeIbPortFlap,
					EventReason: "mlx5_0 port 1 flap since " + timeNow.Format(time.RFC3339),
				},
				{
					Time: timeNow,
					Port: infiniband.IBPort{
						Device: "mlx5_1",
						Port:   1,
					},
					EventType:   infinibandstore.EventTypeIbPortDrop,
					EventReason: "mlx5_1 port 1 drop since " + timeNow.Format(time.RFC3339),
				},
			},
		}

		c := &component{
			ctx:    context.Background(),
			cancel: func() {},

			checkInterval:  time.Minute,
			requestTimeout: 15 * time.Second,

			nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},

			eventBucket:  bucket,
			ibPortsStore: mockStore,

			getTimeNowFunc: func() time.Time {
				return timeNow
			},
			getThresholdsFunc: func() infiniband.ExpectedPortStates {
				return threshold
			},
			getClassDevicesFunc: func() (infinibandclass.Devices, error) {
				return infinibandclass.LoadDevices(classRootDir)
			},
		}

		cr := c.Check()

		// The component should detect flap/drop events and include the EventReason in the summary
		summary := cr.Summary()
		assert.Contains(t, summary, "mlx5_0 port 1 flap since")
		assert.Contains(t, summary, "mlx5_1 port 1 drop since")
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	})

	// Test multiple events of the same type
	t.Run("multiple_same_type_events", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime.Add(5 * time.Minute)

		// Create mock store with multiple flap events
		mockStore := &mockIBPortsStore{
			events: []infinibandstore.Event{
				{
					Time: timeNow,
					Port: infiniband.IBPort{
						Device: "mlx5_0",
						Port:   1,
					},
					EventType:   infinibandstore.EventTypeIbPortFlap,
					EventReason: "mlx5_0 port 1 flap since " + timeNow.Format(time.RFC3339),
				},
				{
					Time: timeNow,
					Port: infiniband.IBPort{
						Device: "mlx5_1",
						Port:   1,
					},
					EventType:   infinibandstore.EventTypeIbPortFlap,
					EventReason: "mlx5_1 port 1 flap since " + timeNow.Format(time.RFC3339),
				},
			},
		}

		c := &component{
			ctx:    context.Background(),
			cancel: func() {},

			checkInterval:  time.Minute,
			requestTimeout: 15 * time.Second,

			nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},

			eventBucket:  bucket,
			ibPortsStore: mockStore,

			getTimeNowFunc: func() time.Time {
				return timeNow
			},
			getThresholdsFunc: func() infiniband.ExpectedPortStates {
				return threshold
			},
			getClassDevicesFunc: func() (infinibandclass.Devices, error) {
				return infinibandclass.LoadDevices(classRootDir)
			},
		}

		cr := c.Check()

		// Should have both flap events in the summary
		summary := cr.Summary()
		assert.Contains(t, summary, "mlx5_0 port 1 flap since")
		assert.Contains(t, summary, "mlx5_1 port 1 flap since")
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	})

	// Test unknown event type
	t.Run("unknown_event_type", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime.Add(10 * time.Minute)

		// Create mock store with unknown event type
		mockStore := &mockIBPortsStore{
			events: []infinibandstore.Event{
				{
					Time: timeNow,
					Port: infiniband.IBPort{
						Device: "mlx5_0",
						Port:   1,
					},
					EventType:   "unknown_event_type",
					EventReason: "unknown event reason",
				},
				{
					Time: timeNow,
					Port: infiniband.IBPort{
						Device: "mlx5_1",
						Port:   1,
					},
					EventType:   infinibandstore.EventTypeIbPortDrop,
					EventReason: "mlx5_1 port 1 drop since " + timeNow.Format(time.RFC3339),
				},
			},
		}

		c := &component{
			ctx:    context.Background(),
			cancel: func() {},

			checkInterval:  time.Minute,
			requestTimeout: 15 * time.Second,

			nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},

			eventBucket:  bucket,
			ibPortsStore: mockStore,

			getTimeNowFunc: func() time.Time {
				return timeNow
			},
			getThresholdsFunc: func() infiniband.ExpectedPortStates {
				return threshold
			},
			getClassDevicesFunc: func() (infinibandclass.Devices, error) {
				return infinibandclass.LoadDevices(classRootDir)
			},
		}

		cr := c.Check()

		// Should only include the known event type (drop), not the unknown one
		summary := cr.Summary()
		assert.Contains(t, summary, "mlx5_1 port 1 drop since")
		assert.NotContains(t, summary, "unknown event reason")
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	})

	// Test empty EventReason
	t.Run("empty_event_reason", func(t *testing.T) {
		threshold := infiniband.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		timeNow := baseTime.Add(15 * time.Minute)

		// Create mock store with empty EventReason
		mockStore := &mockIBPortsStore{
			events: []infinibandstore.Event{
				{
					Time: timeNow,
					Port: infiniband.IBPort{
						Device: "mlx5_0",
						Port:   1,
					},
					EventType:   infinibandstore.EventTypeIbPortFlap,
					EventReason: "", // Empty reason
				},
			},
		}

		c := &component{
			ctx:    context.Background(),
			cancel: func() {},

			checkInterval:  time.Minute,
			requestTimeout: 15 * time.Second,

			nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},

			eventBucket:  bucket,
			ibPortsStore: mockStore,

			getTimeNowFunc: func() time.Time {
				return timeNow
			},
			getThresholdsFunc: func() infiniband.ExpectedPortStates {
				return threshold
			},
			getClassDevicesFunc: func() (infinibandclass.Devices, error) {
				return infinibandclass.LoadDevices(classRootDir)
			},
		}

		cr := c.Check()

		// Should be unhealthy but summary should handle empty reason gracefully
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
		// The summary should still contain the reason (even if empty)
		assert.Contains(t, cr.Summary(), ";")
	})
}
