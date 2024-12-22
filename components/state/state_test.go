package state

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	err = CreateTableMachineMetadata(context.Background(), db)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	return db
}

func TestCreateMachineIDIfNotExist(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Test 1: Create new machine ID
	t.Run("create new machine id", func(t *testing.T) {
		machineID, err := CreateMachineIDIfNotExist(context.Background(), db, "test-uid")
		if err != nil {
			t.Errorf("failed to create machine ID: %v", err)
		}
		if machineID != "test-uid" {
			t.Errorf("expected machine ID 'test-uid', got '%s'", machineID)
		}
	})

	// Test 2: Reuse existing machine ID
	t.Run("reuse existing machine id", func(t *testing.T) {
		machineID1, err := CreateMachineIDIfNotExist(context.Background(), db, "test-uid2")
		if err != nil {
			t.Errorf("failed to create first machine ID: %v", err)
		}

		machineID2, err := CreateMachineIDIfNotExist(context.Background(), db, "different-uid")
		if err != nil {
			t.Errorf("failed to get existing machine ID: %v", err)
		}

		if machineID1 != machineID2 {
			t.Errorf("expected same machine ID, got '%s' and '%s'", machineID1, machineID2)
		}
	})
}

func TestReadMachineID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Test 1: Read from empty database
	t.Run("read from empty database", func(t *testing.T) {
		machineID, err := ReadMachineID(ctx, db)
		if err == nil {
			t.Error("expected error for empty database")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}
		if machineID != "" {
			t.Errorf("expected empty machine ID, got '%s'", machineID)
		}
	})

	// Test 2: Read after creating machine ID
	t.Run("read after creating machine ID", func(t *testing.T) {
		expectedID := "test-machine-id"
		_, err := CreateMachineIDIfNotExist(ctx, db, expectedID)
		if err != nil {
			t.Fatalf("failed to create machine ID: %v", err)
		}

		machineID, err := ReadMachineID(ctx, db)
		if err != nil {
			t.Errorf("failed to read machine ID: %v", err)
		}
		if machineID != expectedID {
			t.Errorf("expected machine ID '%s', got '%s'", expectedID, machineID)
		}
	})

	// Test 3: Read after multiple creations (should return first ID)
	t.Run("read after multiple creations", func(t *testing.T) {
		db := setupTestDB(t) // fresh database
		defer db.Close()

		firstID := "first-id"
		_, err := CreateMachineIDIfNotExist(ctx, db, firstID)
		if err != nil {
			t.Fatalf("failed to create first machine ID: %v", err)
		}

		// Try to create with different ID
		_, err = CreateMachineIDIfNotExist(ctx, db, "second-id")
		if err != nil {
			t.Fatalf("failed to create second machine ID: %v", err)
		}

		// Read should return the first ID
		machineID, err := ReadMachineID(ctx, db)
		if err != nil {
			t.Errorf("failed to read machine ID: %v", err)
		}
		if machineID != firstID {
			t.Errorf("expected first machine ID '%s', got '%s'", firstID, machineID)
		}
	})
}

func TestLoginInfo(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	machineID := "test-machine"

	// Create machine ID first
	_, err := CreateMachineIDIfNotExist(ctx, db, machineID)
	if err != nil {
		t.Fatalf("failed to create machine ID: %v", err)
	}

	// Test 1: Get non-existent token
	t.Run("get non-existent token", func(t *testing.T) {
		loginInfo, err := GetLoginInfo(ctx, db, machineID)
		if err != nil {
			t.Errorf("expected no error for non-existent token, got error: %v", err)
		}
		if loginInfo != nil {
			t.Errorf("expected nil login info, got %+v", loginInfo)
		}
	})

	// Test 2: Update and get token
	t.Run("update and get token", func(t *testing.T) {
		testToken := "test-token"
		beforeUpdate := time.Now().UTC()

		time.Sleep(time.Second + 200*time.Millisecond)

		err := UpdateLoginInfo(ctx, db, machineID, testToken)
		if err != nil {
			t.Errorf("failed to update login info: %v", err)
		}

		loginInfo, err := GetLoginInfo(ctx, db, machineID)
		if err != nil {
			t.Errorf("failed to get login info: %v", err)
		}
		if loginInfo == nil {
			t.Error("expected login info, got nil")
		}
		if loginInfo.Token != testToken {
			t.Errorf("expected token '%s', got '%s'", testToken, loginInfo.Token)
		}
		if loginInfo.LoginTime.Before(beforeUpdate) {
			t.Errorf("expected timestamp after %v, got %v", beforeUpdate, loginInfo.LoginTime)
		}
	})

	// Test 3: Get login info with invalid machine ID
	t.Run("get login info with invalid machine ID", func(t *testing.T) {
		loginInfo, err := GetLoginInfo(ctx, db, "invalid-machine-id")
		if err != nil {
			t.Error("expected no error for invalid machine ID")
		}
		if loginInfo != nil {
			t.Errorf("expected nil login info for invalid machine ID, got %+v", loginInfo)
		}
	})

	// Test 4: Multiple updates and gets
	t.Run("multiple updates and gets", func(t *testing.T) {
		firstToken := "first-token"
		secondToken := "second-token"

		// Update first token
		err := UpdateLoginInfo(ctx, db, machineID, firstToken)
		if err != nil {
			t.Errorf("failed to update first token: %v", err)
		}

		// Get and verify first token
		loginInfo, err := GetLoginInfo(ctx, db, machineID)
		if err != nil {
			t.Errorf("failed to get first login info: %v", err)
		}
		if loginInfo.Token != firstToken {
			t.Errorf("expected first token '%s', got '%s'", firstToken, loginInfo.Token)
		}

		// Update second token
		err = UpdateLoginInfo(ctx, db, machineID, secondToken)
		if err != nil {
			t.Errorf("failed to update second token: %v", err)
		}

		// Get and verify second token
		loginInfo, err = GetLoginInfo(ctx, db, machineID)
		if err != nil {
			t.Errorf("failed to get second login info: %v", err)
		}
		if loginInfo.Token != secondToken {
			t.Errorf("expected second token '%s', got '%s'", secondToken, loginInfo.Token)
		}
	})

	// Test 5: Empty token update
	t.Run("empty token update", func(t *testing.T) {
		err := UpdateLoginInfo(ctx, db, machineID, "")
		if err != nil {
			t.Errorf("failed to update with empty token: %v", err)
		}

		loginInfo, err := GetLoginInfo(ctx, db, machineID)
		if err != nil {
			t.Errorf("failed to get login info after empty update: %v", err)
		}
		if loginInfo == nil {
			t.Error("expected non-nil login info after empty update")
		}
		if loginInfo.Token != "" {
			t.Errorf("expected empty token, got '%s'", loginInfo.Token)
		}
	})
}

func TestComponents(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	machineID := "test-machine"

	// Create machine ID first
	_, err := CreateMachineIDIfNotExist(ctx, db, machineID)
	if err != nil {
		t.Fatalf("failed to create machine ID: %v", err)
	}

	// Test 1: Get non-existent components
	t.Run("get non-existent components", func(t *testing.T) {
		components, err := GetComponents(ctx, db, machineID)
		if err == nil {
			t.Error("expected error for non-existent components")
		}
		if components != "" {
			t.Errorf("expected empty components, got '%s'", components)
		}
	})

	// Test 2: Update and get components
	t.Run("update and get components", func(t *testing.T) {
		testComponents := `{"component1": "value1", "component2": "value2"}`
		err := UpdateComponents(ctx, db, machineID, testComponents)
		if err != nil {
			t.Errorf("failed to update components: %v", err)
		}

		components, err := GetComponents(ctx, db, machineID)
		if err != nil {
			t.Errorf("failed to get components: %v", err)
		}
		if components != testComponents {
			t.Errorf("expected components '%s', got '%s'", testComponents, components)
		}
	})
}

func TestCreateTableMachineMetadata(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Test creating table twice (should not error)
	err := CreateTableMachineMetadata(context.Background(), db)
	if err != nil {
		t.Errorf("failed to create table second time: %v", err)
	}

	// Test table structure
	rows, err := db.Query(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name=?`,
		TableNameMachineMetadata,
	)
	if err != nil {
		t.Fatalf("failed to query table: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Error("table was not created")
	}
}

func TestMachineIDWithEmptyUID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Test with empty UID (should generate a UUID)
	machineID, err := CreateMachineIDIfNotExist(context.Background(), db, "")
	if err != nil {
		t.Errorf("failed to create machine ID with empty UID: %v", err)
	}
	if machineID == "" {
		t.Error("expected non-empty machine ID")
	}
}
