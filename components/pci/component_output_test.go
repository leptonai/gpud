package pci

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/common"
	events_db "github.com/leptonai/gpud/components/db"
	query_config "github.com/leptonai/gpud/components/query/config"
	"github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/pci"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestGet(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	if err != nil {
		t.Fatalf("failed to create events store: %v", err)
	}
	defer eventsStore.Close()

	getFunc := CreateGet(eventsStore)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err = getFunc(ctx); err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
}

func TestCreateGet(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	tests := []struct {
		name          string
		setupVirtEnv  func()
		setupEvents   func(store events_db.Store) error
		expectedError bool
		skipInsertion bool
	}{
		{
			name: "KVM environment should return nil",
			setupVirtEnv: func() {
				currentVirtEnv = host.VirtualizationEnvironment{
					IsKVM: true,
					Type:  "kvm",
				}
			},
			skipInsertion: true,
		},
		{
			name: "Unknown virtualization environment should return nil",
			setupVirtEnv: func() {
				currentVirtEnv = host.VirtualizationEnvironment{
					IsKVM: false,
					Type:  "",
				}
			},
			skipInsertion: true,
		},
		{
			name: "Recent event within 24h should skip check",
			setupVirtEnv: func() {
				currentVirtEnv = host.VirtualizationEnvironment{
					IsKVM: false,
					Type:  "none",
				}
			},
			setupEvents: func(store events_db.Store) error {
				event := components.Event{
					Time:    metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
					Name:    "acs_enabled",
					Type:    common.EventTypeWarning,
					Message: "test event",
				}
				return store.Insert(context.Background(), event)
			},
			skipInsertion: true,
		},
		{
			name: "Baremetal environment should check ACS",
			setupVirtEnv: func() {
				currentVirtEnv = host.VirtualizationEnvironment{
					IsKVM: false,
					Type:  "none",
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
			require.NoError(t, err)
			defer eventsStore.Close()

			if tt.setupVirtEnv != nil {
				tt.setupVirtEnv()
			}

			if tt.setupEvents != nil {
				err := tt.setupEvents(eventsStore)
				require.NoError(t, err)
			}

			getFunc := CreateGet(eventsStore)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err = getFunc(ctx)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if !tt.skipInsertion {
				// Verify event insertion
				lastEvent, err := eventsStore.Latest(ctx)
				assert.NoError(t, err)
				if lastEvent != nil {
					assert.Equal(t, "acs_enabled", lastEvent.Name)
					assert.Equal(t, common.EventTypeWarning, lastEvent.Type)
				}
			}
		})
	}
}

func TestDefaultPoller(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	require.NoError(t, err)
	defer eventsStore.Close()

	// Test initial state
	assert.Nil(t, getDefaultPoller())

	// Test setting default poller
	cfg := Config{
		Query: query_config.Config{},
	}
	setDefaultPoller(cfg, eventsStore)
	assert.NotNil(t, getDefaultPoller())

	// Test that calling setDefaultPoller again doesn't change the poller (sync.Once)
	originalPoller := getDefaultPoller()
	setDefaultPoller(cfg, eventsStore)
	assert.Equal(t, originalPoller, getDefaultPoller())
}

func TestCreateGetWithFailedEventStore(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	// Setup a virtualization environment that would proceed with checks
	currentVirtEnv = host.VirtualizationEnvironment{
		IsKVM: false,
		Type:  "none",
	}

	// Create an events store that will fail on Latest()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	require.NoError(t, err)

	// Close the database to force errors
	cleanup()

	getFunc := CreateGet(eventsStore)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = getFunc(ctx)
	assert.Error(t, err)
}

func TestCreateGetWithContextTimeout(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	require.NoError(t, err)
	defer eventsStore.Close()

	getFunc := CreateGet(eventsStore)

	// create an already canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = getFunc(ctx)
	assert.Error(t, err)
}

func TestCreateGetWithEventStoreInsertError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	// Setup a virtualization environment that would proceed with checks
	currentVirtEnv = host.VirtualizationEnvironment{
		IsKVM: false,
		Type:  "none",
	}

	// Create an events store that will fail on Insert
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	require.NoError(t, err)

	// Close the database to force errors on insert
	cleanup()

	getFunc := CreateGet(eventsStore)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = getFunc(ctx)
	assert.Error(t, err)
}

func TestCreateGetWithOldEvent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	require.NoError(t, err)
	defer eventsStore.Close()

	// setup virtualization environment
	currentVirtEnv = host.VirtualizationEnvironment{
		IsKVM: false,
		Type:  "none",
	}

	// insert an old event (less than 24 hours ago)
	oldEvent := components.Event{
		Time:    metav1.Time{Time: time.Now().Add(-10 * time.Hour)},
		Name:    "acs_enabled",
		Type:    common.EventTypeWarning,
		Message: "old test event",
	}
	err = eventsStore.Insert(context.Background(), oldEvent)
	require.NoError(t, err)

	getFunc := CreateGet(eventsStore)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = getFunc(ctx)
	assert.NoError(t, err)

	lastEvent, err := eventsStore.Latest(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, lastEvent)
	assert.Equal(t, "acs_enabled", lastEvent.Name)
	assert.Equal(t, common.EventTypeWarning, lastEvent.Type)
}

func TestCreateGetWithEmptyEventStore(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	require.NoError(t, err)
	defer eventsStore.Close()

	// Setup virtualization environment
	currentVirtEnv = host.VirtualizationEnvironment{
		IsKVM: false,
		Type:  "none",
	}

	getFunc := CreateGet(eventsStore)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = getFunc(ctx)
	assert.NoError(t, err)

	// Verify that an event was created in empty store
	lastEvent, err := eventsStore.Latest(ctx)
	assert.NoError(t, err)
	if lastEvent != nil {
		assert.Equal(t, "acs_enabled", lastEvent.Name)
		assert.Equal(t, common.EventTypeWarning, lastEvent.Type)
	}
}

func TestCreateGetWithMultipleEvents(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	require.NoError(t, err)
	defer eventsStore.Close()

	// Setup virtualization environment
	currentVirtEnv = host.VirtualizationEnvironment{
		IsKVM: false,
		Type:  "none",
	}

	// Insert multiple events with different timestamps
	events := []components.Event{
		{
			Time:    metav1.Time{Time: time.Now().Add(-48 * time.Hour)},
			Name:    "acs_enabled",
			Type:    common.EventTypeWarning,
			Message: "old event 1",
		},
		{
			Time:    metav1.Time{Time: time.Now().Add(-36 * time.Hour)},
			Name:    "acs_enabled",
			Type:    common.EventTypeWarning,
			Message: "old event 2",
		},
	}

	for _, event := range events {
		err = eventsStore.Insert(context.Background(), event)
		require.NoError(t, err)
	}

	getFunc := CreateGet(eventsStore)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = getFunc(ctx)
	assert.NoError(t, err)

	lastEvent, err := eventsStore.Latest(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, lastEvent)
	assert.Equal(t, "acs_enabled", lastEvent.Name)
}

func TestCreateGetWithDifferentVirtEnvTypes(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	tests := []struct {
		name     string
		virtType string
		isKVM    bool
		checkACS bool
	}{
		{
			name:     "Docker environment",
			virtType: "docker",
			isKVM:    false,
			checkACS: true,
		},
		{
			name:     "LXC environment",
			virtType: "lxc",
			isKVM:    false,
			checkACS: true,
		},
		{
			name:     "Xen environment",
			virtType: "xen",
			isKVM:    false,
			checkACS: true,
		},
		{
			name:     "VMware environment",
			virtType: "vmware",
			isKVM:    false,
			checkACS: true,
		},
		{
			name:     "Baremetal environment",
			virtType: "none",
			isKVM:    false,
			checkACS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
			require.NoError(t, err)
			defer eventsStore.Close()

			currentVirtEnv = host.VirtualizationEnvironment{
				IsKVM: tt.isKVM,
				Type:  tt.virtType,
			}

			getFunc := CreateGet(eventsStore)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err = getFunc(ctx)
			assert.NoError(t, err)

			if tt.checkACS {
				lastEvent, err := eventsStore.Latest(ctx)
				assert.NoError(t, err)
				if lastEvent != nil {
					assert.Equal(t, "acs_enabled", lastEvent.Name)
					assert.Equal(t, common.EventTypeWarning, lastEvent.Type)
					assert.Contains(t, lastEvent.Message, tt.virtType)
				}
			}
		})
	}
}

func TestCreateGetMetrics(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	tests := []struct {
		name          string
		setupVirtEnv  func()
		injectError   bool
		expectSuccess bool
	}{
		{
			name: "Success case sets success metric",
			setupVirtEnv: func() {
				currentVirtEnv = host.VirtualizationEnvironment{
					IsKVM: true,
					Type:  "kvm",
				}
			},
			injectError:   false,
			expectSuccess: true,
		},
		{
			name: "Error case sets failure metric",
			setupVirtEnv: func() {
				currentVirtEnv = host.VirtualizationEnvironment{
					IsKVM: false,
					Type:  "none",
				}
			},
			injectError:   true,
			expectSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
			require.NoError(t, err)

			if tt.setupVirtEnv != nil {
				tt.setupVirtEnv()
			}

			if tt.injectError {
				// Close DB to force error
				cleanup()
			} else {
				defer eventsStore.Close()
			}

			getFunc := CreateGet(eventsStore)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err = getFunc(ctx)
			if tt.injectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateGetEarlyReturns(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	tests := []struct {
		name          string
		setupVirtEnv  func()
		expectMetrics bool
	}{
		{
			name: "KVM environment returns early",
			setupVirtEnv: func() {
				currentVirtEnv = host.VirtualizationEnvironment{
					IsKVM: true,
					Type:  "kvm",
				}
			},
			expectMetrics: true,
		},
		{
			name: "Empty virtualization type returns early",
			setupVirtEnv: func() {
				currentVirtEnv = host.VirtualizationEnvironment{
					IsKVM: false,
					Type:  "",
				}
			},
			expectMetrics: true,
		},
		{
			name: "Non-KVM with valid type continues execution",
			setupVirtEnv: func() {
				currentVirtEnv = host.VirtualizationEnvironment{
					IsKVM: false,
					Type:  "none",
				}
			},
			expectMetrics: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
			require.NoError(t, err)
			defer eventsStore.Close()

			if tt.setupVirtEnv != nil {
				tt.setupVirtEnv()
			}

			getFunc := CreateGet(eventsStore)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err = getFunc(ctx)
			assert.NoError(t, err)

			// For early returns, verify no events were created
			if currentVirtEnv.IsKVM || currentVirtEnv.Type == "" {
				lastEvent, err := eventsStore.Latest(ctx)
				assert.NoError(t, err)
				assert.Nil(t, lastEvent, "Early return should not create events")
			}
		})
	}
}

func TestCreateGetWithNilEventStore(t *testing.T) {
	currentVirtEnv = host.VirtualizationEnvironment{
		IsKVM: false,
		Type:  "none",
	}

	getFunc := CreateGet(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := getFunc(ctx)
	assert.Equal(t, err, ErrNoEventStore)
}

func TestCreateEvent(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		devices []pci.Device
		virtEnv host.VirtualizationEnvironment
		want    *components.Event
	}{
		{
			name:    "no devices",
			devices: []pci.Device{},
			virtEnv: host.VirtualizationEnvironment{Type: "none"},
			want:    nil,
		},
		{
			name: "devices without ACS",
			devices: []pci.Device{
				{ID: "dev1", AccessControlService: nil},
				{ID: "dev2", AccessControlService: nil},
			},
			virtEnv: host.VirtualizationEnvironment{Type: "none"},
			want:    nil,
		},
		{
			name: "devices with ACS but SrcValid false",
			devices: []pci.Device{
				{
					ID: "dev1",
					AccessControlService: &pci.AccessControlService{
						ACSCtl: pci.ACS{SrcValid: false},
					},
				},
			},
			virtEnv: host.VirtualizationEnvironment{Type: "none"},
			want:    nil,
		},
		{
			name: "devices with ACS and SrcValid true",
			devices: []pci.Device{
				{
					ID: "dev1",
					AccessControlService: &pci.AccessControlService{
						ACSCtl: pci.ACS{SrcValid: true},
					},
				},
				{
					ID: "dev2",
					AccessControlService: &pci.AccessControlService{
						ACSCtl: pci.ACS{SrcValid: true},
					},
				},
			},
			virtEnv: host.VirtualizationEnvironment{Type: "none"},
			want: &components.Event{
				Time:    metav1.Time{Time: testTime},
				Name:    "acs_enabled",
				Type:    common.EventTypeWarning,
				Message: `host virt env is "none", ACS is enabled on the following PCI devices: dev1, dev2`,
			},
		},
		{
			name: "mixed devices",
			devices: []pci.Device{
				{ID: "dev1", AccessControlService: nil},
				{
					ID: "dev2",
					AccessControlService: &pci.AccessControlService{
						ACSCtl: pci.ACS{SrcValid: true},
					},
				},
				{
					ID: "dev3",
					AccessControlService: &pci.AccessControlService{
						ACSCtl: pci.ACS{SrcValid: false},
					},
				},
			},
			virtEnv: host.VirtualizationEnvironment{Type: "kvm"},
			want: &components.Event{
				Time:    metav1.Time{Time: testTime},
				Name:    "acs_enabled",
				Type:    common.EventTypeWarning,
				Message: `host virt env is "kvm", ACS is enabled on the following PCI devices: dev2`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the virtualization environment for this test
			currentVirtEnv = tt.virtEnv

			got := createEvent(testTime, tt.devices)
			assert.Equal(t, tt.want, got)
		})
	}
}
