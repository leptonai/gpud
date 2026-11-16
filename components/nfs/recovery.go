package nfs

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
)

func (c *component) checkRecovery(cr *checkResult, unresolved, before eventstore.Events) {
	// Soft RPC timeouts do not set RPC_CALL_MAJORSEEN, so later successful I/O
	// need not emit "server OK". Revalidate instead of waiting for that log pair.
	servers := make(map[string]bool)
	for _, ev := range unresolved {
		// Ordinary I/O does not prove that lost locks or blocked writeback recovered.
		// Only a server-not-responding warning can be superseded by this probe.
		if ev.Name != eventNFSServerNotResponding {
			return
		}
		server := nfsServerFromEventMessage(ev.Message)
		if server == "" {
			return
		}
		servers[server] = true
	}
	req := pkgnfschecker.RecoveryRequest{Configs: slices.Clone(c.getGroupConfigsFunc()), HostRoot: c.nfsHostRoot}
	if len(req.Configs) == 0 {
		cr.reason += "; automatic recovery requires configured NFS probes"
		return
	}
	for server := range servers {
		req.Servers = append(req.Servers, server)
	}
	sort.Strings(req.Servers)
	probe := c.recoverNFS
	if probe == nil {
		probe = c.recoveryRunner.Run
	}
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	if err := probe(ctx, req); err != nil {
		cr.reason += fmt.Sprintf("; recovery verification failed: %v", err)
		return
	}
	if err := ctx.Err(); err != nil {
		cr.reason += "; recovery verification canceled"
		return
	}
	// Do not purge history or cache a Healthy verdict. Each check must obtain
	// fresh I/O evidence, also after daemon restart. Re-read events so a warning
	// observed while probing cannot be cleared by an older successful operation.
	events, err := c.eventBucket.Get(ctx, cr.ts.Add(-c.lookbackPeriod))
	if err != nil {
		cr.reason += "; failed to recheck NFS events"
		return
	}
	if !reflect.DeepEqual(req.Configs, c.getGroupConfigsFunc()) {
		cr.reason += "; NFS probe configuration changed during verification"
		return
	}
	current, _ := collectNFSHangEvents(events)
	for _, ev := range current {
		found := false
		for _, old := range before {
			if ev.Name == old.Name && ev.Message == old.Message && ev.Time.Equal(old.Time) {
				found = true
				break
			}
		}
		if !found {
			cr.reason += "; new NFS issue observed during recovery verification"
			return
		}
	}
	if ctx.Err() != nil {
		cr.reason += "; recovery verification canceled"
		return
	}
	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "NFS recovery verified by fresh write, fsync, read and removal on configured mounts"
}
