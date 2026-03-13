package nfs

import (
	"sync"

	"github.com/leptonai/gpud/pkg/log"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
)

var (
	defaultConfigsMu sync.RWMutex
	defaultConfigs   = make(pkgnfschecker.Configs, 0)
)

// GetDefaultConfigs returns the current default NFS checker configs.
func GetDefaultConfigs() pkgnfschecker.Configs {
	defaultConfigsMu.RLock()
	defer defaultConfigsMu.RUnlock()

	return defaultConfigs
}

// SetDefaultConfigs replaces the default NFS checker configs.
func SetDefaultConfigs(cfgs pkgnfschecker.Configs) {
	log.Logger.Infow("setting default nfs checker configs", "count", len(cfgs))

	defaultConfigsMu.Lock()
	defer defaultConfigsMu.Unlock()
	defaultConfigs = cfgs
}
