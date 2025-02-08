package systemd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-systemd/v22/dbus"
)

type dbusConn interface {
	Close()
	Connected() bool
	GetUnitPropertiesContext(ctx context.Context, unit string) (map[string]interface{}, error)
}

type DbusConn struct {
	conn dbusConn
}

// Caller should explicitly close the connection by calling Close() on returned connection object
func NewDbusConn(ctx context.Context) (*DbusConn, error) {
	dbc, err := dbus.NewWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed connect to systemd dbus: %w", err)
	}
	return &DbusConn{
		conn: dbc,
	}, nil
}

func (c *DbusConn) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *DbusConn) IsActive(ctx context.Context, unitName string) (bool, error) {
	if c.conn == nil {
		return false, errors.New("connection not initialized")
	}
	if !c.conn.Connected() {
		return false, fmt.Errorf("connection disconnected")
	}

	formattedUnitName := normalizeServiceUnitName(unitName)
	props, err := c.conn.GetUnitPropertiesContext(ctx, formattedUnitName)
	if err != nil {
		return false, fmt.Errorf("unable to get unit properties for %s: %w", formattedUnitName, err)
	}

	return checkActiveState(props, formattedUnitName)
}

// normalizeServiceUnitName ensures the unit name has the correct suffix
func normalizeServiceUnitName(unitName string) string {
	if !strings.HasSuffix(unitName, ".target") && !strings.HasSuffix(unitName, ".service") {
		return fmt.Sprintf("%s.service", unitName)
	}
	return unitName
}

// checkActiveState checks if a unit is active based on its properties
func checkActiveState(props map[string]interface{}, unitName string) (bool, error) {
	activeState, ok := props["ActiveState"]
	if !ok {
		return false, fmt.Errorf("ActiveState property not found for unit %s", unitName)
	}
	s, ok := activeState.(string)
	if !ok {
		return false, fmt.Errorf("ActiveState property is not a string for unit %s", unitName)
	}
	return s == "active", nil
}
