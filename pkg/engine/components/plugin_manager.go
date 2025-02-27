package components

import (
	"context"
	"sync"
	"time"

	extism "github.com/extism/go-sdk"
	"github.com/ignitionstack/ignition/pkg/engine"
)

type PluginManager struct {
	plugins        map[string]*extism.Plugin
	pluginLastUsed map[string]time.Time
	pluginsMux     sync.RWMutex
	ttlDuration    time.Duration
	cleanupTicker  *time.Ticker
	logger         engine.Logger
}

func NewPluginManager(logger engine.Logger) *PluginManager {
	return &PluginManager{
		plugins:        make(map[string]*extism.Plugin),
		pluginLastUsed: make(map[string]time.Time),
		ttlDuration:    30 * time.Minute,
		logger:         logger,
	}
}

func (pm *PluginManager) StartCleanup(ctx context.Context) {
	pm.logger.Printf("Starting plugin cleanup goroutine")
	pm.cleanupTicker = time.NewTicker(5 * time.Minute)

	go func() {
		for {
			select {
			case <-pm.cleanupTicker.C:
				pm.logger.Printf("Running plugin cleanup")
				pm.cleanupUnusedPlugins()

			case <-ctx.Done():
				pm.logger.Printf("Cleanup goroutine received shutdown signal")
				if pm.cleanupTicker != nil {
					pm.cleanupTicker.Stop()
				}
				return
			}
		}
	}()
}

func (pm *PluginManager) cleanupUnusedPlugins() {
	pm.pluginsMux.Lock()
	defer pm.pluginsMux.Unlock()

	now := time.Now()
	for key, lastUsed := range pm.pluginLastUsed {
		if now.Sub(lastUsed) > pm.ttlDuration {
			if plugin, exists := pm.plugins[key]; exists {
				plugin.Close(context.TODO())
				delete(pm.plugins, key)
				delete(pm.pluginLastUsed, key)
				pm.logger.Printf("Plugin %s unloaded due to inactivity", key)
			}
		}
	}
}

func (pm *PluginManager) GetPlugin(key string) (*extism.Plugin, bool) {
	pm.pluginsMux.RLock()
	plugin, ok := pm.plugins[key]
	if ok {
		pm.pluginLastUsed[key] = time.Now()
	}
	pm.pluginsMux.RUnlock()

	return plugin, ok
}

func (pm *PluginManager) StorePlugin(key string, plugin *extism.Plugin) {
	pm.pluginsMux.Lock()
	defer pm.pluginsMux.Unlock()

	pm.plugins[key] = plugin
	pm.pluginLastUsed[key] = time.Now()

	pm.logger.Printf("Plugin %s loaded and stored", key)
}

func (pm *PluginManager) RemovePlugin(key string) bool {
	pm.pluginsMux.Lock()
	defer pm.pluginsMux.Unlock()

	plugin, exists := pm.plugins[key]
	if exists {
		plugin.Close(context.TODO())
		delete(pm.plugins, key)
		delete(pm.pluginLastUsed, key)
		return true
	}

	return false
}

func (pm *PluginManager) IsPluginLoaded(key string) bool {
	pm.pluginsMux.RLock()
	_, exists := pm.plugins[key]
	pm.pluginsMux.RUnlock()

	return exists
}

func (pm *PluginManager) Shutdown() {
	if pm.cleanupTicker != nil {
		pm.cleanupTicker.Stop()
	}

	pm.pluginsMux.Lock()
	defer pm.pluginsMux.Unlock()

	for key, plugin := range pm.plugins {
		plugin.Close(context.TODO())
		delete(pm.plugins, key)
	}
}
