package components

import (
	"context"
	"fmt"
	"sync"
	"time"

	extism "github.com/extism/go-sdk"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/ignitionstack/ignition/pkg/registry"
)

type PluginManager struct {
	plugins        map[string]*extism.Plugin
	pluginLastUsed map[string]time.Time
	pluginsMux     sync.RWMutex
	ttlDuration    time.Duration
	cleanupTicker  *time.Ticker
	logger         logging.Logger

	// Using separate mutexes for different maps to reduce contention
	pluginDigests       map[string]string
	pluginDigestsMux    sync.RWMutex
	pluginConfigs       map[string]map[string]string
	pluginConfigsMux    sync.RWMutex
	previouslyLoaded    map[string]bool
	previouslyLoadedMux sync.RWMutex

	logStore *logging.FunctionLogStore
}

type PluginOptions struct {
	TTL              time.Duration
	CleanupInterval  time.Duration
	LogStoreCapacity int
}

func DefaultPluginOptions() PluginOptions {
	return PluginOptions{
		TTL:              10 * time.Minute,
		CleanupInterval:  5 * time.Minute,
		LogStoreCapacity: 1000,
	}
}

func NewPluginManager(logger logging.Logger, options PluginOptions) *PluginManager {
	return &PluginManager{
		plugins:          make(map[string]*extism.Plugin),
		pluginLastUsed:   make(map[string]time.Time),
		ttlDuration:      options.TTL,
		logger:           logger,
		pluginDigests:    make(map[string]string),
		pluginConfigs:    make(map[string]map[string]string),
		previouslyLoaded: make(map[string]bool),
		logStore:         logging.NewFunctionLogStore(options.LogStoreCapacity),
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
				pm.logger.Printf("Plugin %s unloaded due to inactivity, preserving configuration for potential reload", key)
				if pm.logStore != nil {
					pm.logStore.AddLog(key, logging.LevelInfo, "Plugin unloaded due to inactivity, preserving configuration for potential reload")
				}
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

func (pm *PluginManager) StorePlugin(key string, plugin *extism.Plugin, digest string, config map[string]string) {
	// Handle plugin map updates with its own lock
	func() {
		pm.pluginsMux.Lock()
		defer pm.pluginsMux.Unlock()

		// If there's an existing plugin, close it first
		if existing, exists := pm.plugins[key]; exists {
			existing.Close(context.TODO())
		}

		pm.plugins[key] = plugin
		pm.pluginLastUsed[key] = time.Now()
	}()

	// Handle digest update with its own lock
	if digest != "" {
		pm.pluginDigestsMux.Lock()
		pm.pluginDigests[key] = digest
		pm.pluginDigestsMux.Unlock()
	}

	// Handle config update with its own lock
	if config != nil {
		// Make a copy of the config
		configCopy := make(map[string]string, len(config))
		for k, v := range config {
			configCopy[k] = v
		}

		pm.pluginConfigsMux.Lock()
		pm.pluginConfigs[key] = configCopy
		pm.pluginConfigsMux.Unlock()
	}

	// Mark the plugin as having been loaded
	pm.previouslyLoadedMux.Lock()
	pm.previouslyLoaded[key] = true
	pm.previouslyLoadedMux.Unlock()

	pm.logger.Printf("Plugin %s loaded and stored", key)
	if pm.logStore != nil {
		pm.logStore.AddLog(key, logging.LevelInfo, "Plugin loaded and stored")
	}
}

func (pm *PluginManager) RemovePlugin(key string) bool {
	pm.pluginsMux.Lock()
	defer pm.pluginsMux.Unlock()

	plugin, exists := pm.plugins[key]
	if exists {
		plugin.Close(context.TODO())
		delete(pm.plugins, key)
		delete(pm.pluginLastUsed, key)
		if pm.logStore != nil {
			pm.logStore.AddLog(key, logging.LevelInfo, "Plugin unloaded but configuration preserved for potential reload")
		}
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

func (pm *PluginManager) WasPreviouslyLoaded(key string) (bool, map[string]string) {
	pm.previouslyLoadedMux.RLock()
	wasLoaded, exists := pm.previouslyLoaded[key]
	pm.previouslyLoadedMux.RUnlock()

	// Get the last known config for this function
	var config map[string]string
	pm.pluginsMux.RLock()
	lastConfig, hasConfig := pm.pluginConfigs[key]
	if hasConfig {
		// Make a copy of the config
		config = make(map[string]string, len(lastConfig))
		for k, v := range lastConfig {
			config[k] = v
		}
	}
	pm.pluginsMux.RUnlock()

	return exists && wasLoaded, config
}

// HasConfigChanged compares stored config with a new config to check for changes
func (pm *PluginManager) HasConfigChanged(key string, newConfig map[string]string) bool {
	pm.pluginConfigsMux.RLock()
	currentConfig, hasConfig := pm.pluginConfigs[key]
	pm.pluginConfigsMux.RUnlock()

	if !hasConfig {
		return true
	}

	if len(currentConfig) != len(newConfig) {
		return true
	}

	for k, v := range newConfig {
		if currentValue, exists := currentConfig[k]; !exists || currentValue != v {
			return true
		}
	}

	for k := range currentConfig {
		if _, exists := newConfig[k]; !exists {
			return true
		}
	}

	return false
}

func (pm *PluginManager) HasDigestChanged(key string, newDigest string) bool {
	pm.pluginDigestsMux.RLock()
	currentDigest, hasDigest := pm.pluginDigests[key]
	pm.pluginDigestsMux.RUnlock()

	return !hasDigest || currentDigest != newDigest
}

func CreatePlugin(wasmBytes []byte, versionInfo *registry.VersionInfo, config map[string]string) (*extism.Plugin, error) {
	manifest := extism.Manifest{
		AllowedHosts: versionInfo.Settings.AllowedUrls,
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
		Config: config,
	}

	pluginConfig := extism.PluginConfig{
		EnableWasi: versionInfo.Settings.Wasi,
	}

	return extism.NewPlugin(context.Background(), manifest, pluginConfig, []extism.HostFunction{})
}

func (pm *PluginManager) GetLogStore() *logging.FunctionLogStore {
	return pm.logStore
}

func (pm *PluginManager) GetPluginDigest(key string) (string, bool) {
	pm.pluginDigestsMux.RLock()
	digest, exists := pm.pluginDigests[key]
	pm.pluginDigestsMux.RUnlock()

	return digest, exists
}

func (pm *PluginManager) GetPluginConfig(key string) (map[string]string, bool) {
	pm.pluginConfigsMux.RLock()
	config, exists := pm.pluginConfigs[key]

	// Make a copy if it exists
	var configCopy map[string]string
	if exists {
		configCopy = make(map[string]string, len(config))
		for k, v := range config {
			configCopy[k] = v
		}
	}
	pm.pluginConfigsMux.RUnlock()

	return configCopy, exists
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

// ListLoadedFunctions returns a list of currently loaded function keys
func (pm *PluginManager) ListLoadedFunctions() []string {
	pm.pluginsMux.RLock()
	defer pm.pluginsMux.RUnlock()

	keys := make([]string, 0, len(pm.plugins))
	for key := range pm.plugins {
		keys = append(keys, key)
	}

	return keys
}

// GetLoadedFunctionCount returns the number of currently loaded functions
func (pm *PluginManager) GetLoadedFunctionCount() int {
	pm.pluginsMux.RLock()
	defer pm.pluginsMux.RUnlock()

	return len(pm.plugins)
}

// Helper function to get standard function key format
func GetFunctionKey(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}
