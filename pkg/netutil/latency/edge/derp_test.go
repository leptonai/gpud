// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package edge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"tailscale.com/net/netcheck"
	"tailscale.com/net/netmon"
	"tailscale.com/net/stun/stuntest"
)

// TestMeasureDERPWithNilMap verifies that calling measureDERP with a nil DERPMap
// results in an error.
func TestMeasureDERPWithNilMap(t *testing.T) {
	ctx := context.Background()

	// Call measureDERP with a nil DERPMap
	result, err := measureDERP(ctx, nil)

	// It should return an error
	assert.Error(t, err)
	assert.Nil(t, result)
}

func newTestClient(t testing.TB) *netcheck.Client {
	c := &netcheck.Client{
		NetMon: netmon.NewStatic(),
		Logf:   t.Logf,
		TimeNow: func() time.Time {
			return time.Unix(1729624521, 0)
		},
	}
	return c
}

func TestBasic(t *testing.T) {
	stunAddr, cleanup := stuntest.Serve(t)
	defer cleanup()

	c := newTestClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := c.Standalone(ctx, "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}

	r, err := c.GetReport(ctx, stuntest.DERPMapOf(stunAddr.String()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !r.UDP {
		t.Error("want UDP")
	}
	if len(r.RegionLatency) != 1 {
		t.Errorf("expected 1 key in DERPLatency; got %+v", r.RegionLatency)
	}
	if _, ok := r.RegionLatency[1]; !ok {
		t.Errorf("expected key 1 in DERPLatency; got %+v", r.RegionLatency)
	}
	if !r.GlobalV4.IsValid() {
		t.Error("expected GlobalV4 set")
	}
	if r.PreferredDERP != 1 {
		t.Errorf("PreferredDERP = %v; want 1", r.PreferredDERP)
	}
	v4Addrs, _ := r.GetGlobalAddrs()
	if len(v4Addrs) != 1 {
		t.Error("expected one global IPv4 address")
	}
	if got, want := v4Addrs[0], r.GlobalV4; got != want {
		t.Errorf("got %v; want %v", got, want)
	}
}
