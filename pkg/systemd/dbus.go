package systemd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-systemd/v22/dbus"
)

type DbusConn struct {
	conn *dbus.Conn
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
	if !strings.HasSuffix(unitName, ".target") && !strings.HasSuffix(unitName, ".service") {
		unitName = fmt.Sprintf("%s.service", unitName)
	}
	props, err := c.conn.GetUnitPropertiesContext(ctx, unitName)
	if err != nil {
		return false, fmt.Errorf("unable to get unit properties for %s: %w", unitName, err)
	}
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
