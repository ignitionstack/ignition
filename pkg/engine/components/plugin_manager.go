package components

import (
	"context"
	"sync"
	"time"

	extism "github.com/extism/go-sdk"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/ignitionstack/ignition/pkg/registry"
)

// The PluginManager interface is defined in interfaces.go

// PluginManagerSettings defines configurable options for the plugin manager.
type PluginManagerSettings struct {
	// How long to keep unused plugins loaded
	TTL time.Duration

	// How often to run the plugin cleanup routine
	CleanupInterval time.Duration
}

// defaultPluginManager implements the PluginManager interface.
type defaultPluginManager struct {
	// Primary plugin storage
	plugins        map[string]*extism.Plugin
	pluginLastUsed map[string]time.Time
	pluginsMux     sync.RWMutex

	// Plugin state and metadata
	pluginDigests       map[string]string
	pluginDigestsMux    sync.RWMutex
	pluginConfigs       map[string]map[string]string
	pluginConfigsMux    sync.RWMutex
	previouslyLoaded    map[string]bool
	previouslyLoadedMux sync.RWMutex
	stoppedFunctions    map[string]bool
	stoppedFunctionsMux sync.RWMutex

	// Configuration
	ttlDuration     time.Duration
	cleanupInterval time.Duration
	cleanupTicker   *time.Ticker

	// Dependencies
	logger   logging.Logger
	logStore *logging.FunctionLogStore
}

// NewPluginManager creates a new plugin manager with the specified settings.
func NewPluginManager(logger logging.Logger, options PluginManagerSettings) PluginManager {
	logStoreCapacity := 1000 // Default value

	return &defaultPluginManager{
		plugins:          make(map[string]*extism.Plugin),
		pluginLastUsed:   make(map[string]time.Time),
		ttlDuration:      options.TTL,
		cleanupInterval:  options.CleanupInterval,
		logger:           logger,
		pluginDigests:    make(map[string]string),
		pluginConfigs:    make(map[string]map[string]string),
		previouslyLoaded: make(map[string]bool),
		stoppedFunctions: make(map[string]bool),
		logStore:         logging.NewFunctionLogStore(logStoreCapacity),
	}
}

func (pm *defaultPluginManager) StartCleanup(ctx context.Context) {
	// Get cleanup interval from options, defaulting to 1 minute
	cleanupInterval := pm.cleanupInterval
	if cleanupInterval == 0 {
		cleanupInterval = time.Minute
	}

	pm.logger.Printf("Starting plugin cleanup goroutine with interval %s", cleanupInterval)
	pm.cleanupTicker = time.NewTicker(cleanupInterval)

	go func() {
		// Run cleanup immediately on start
		pm.cleanupUnusedPlugins()

		for {
			select {
			case <-pm.cleanupTicker.C:
				pm.logger.Printf("Running plugin cleanup (TTL: %s)", pm.ttlDuration)
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

func (pm *defaultPluginManager) cleanupUnusedPlugins() {
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

func (pm *defaultPluginManager) GetPlugin(key string) (*extism.Plugin, bool) {
	pm.pluginsMux.RLock()
	plugin, ok := pm.plugins[key]
	pm.pluginsMux.RUnlock()

	// If the plugin exists, update the last used time with a write lock
	if ok {
		pm.pluginsMux.Lock()
		// Double-check the plugin still exists after getting the write lock
		if _, stillExists := pm.plugins[key]; stillExists {
			pm.pluginLastUsed[key] = time.Now()
		} else {
			ok = false
			plugin = nil
		}
		pm.pluginsMux.Unlock()
	}

	return plugin, ok
}

func (pm *defaultPluginManager) StorePlugin(key string, plugin *extism.Plugin, digest string, config map[string]string) {
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

func (pm *defaultPluginManager) RemovePlugin(key string) bool {
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

// StopFunction permanently stops a function and prevents automatic reloading.
func (pm *defaultPluginManager) StopFunction(key string) bool {
	// First unload the plugin if it's loaded
	removed := pm.RemovePlugin(key)

	// Mark the function as stopped to prevent automatic reload
	pm.stoppedFunctionsMux.Lock()
	pm.stoppedFunctions[key] = true
	pm.stoppedFunctionsMux.Unlock()

	if pm.logStore != nil {
		pm.logStore.AddLog(key, logging.LevelInfo, "Function stopped and will not be automatically reloaded")
	}

	return removed
}

// IsFunctionStopped checks if a function has been explicitly stopped.
func (pm *defaultPluginManager) IsFunctionStopped(key string) bool {
	pm.stoppedFunctionsMux.RLock()
	defer pm.stoppedFunctionsMux.RUnlock()

	stopped, exists := pm.stoppedFunctions[key]
	return exists && stopped
}

// ClearStoppedStatus removes the stopped status from a function, allowing it to be loaded again.
func (pm *defaultPluginManager) ClearStoppedStatus(key string) {
	pm.stoppedFunctionsMux.Lock()
	delete(pm.stoppedFunctions, key)
	pm.stoppedFunctionsMux.Unlock()

	if pm.logStore != nil {
		pm.logStore.AddLog(key, logging.LevelInfo, "Function's stopped status cleared, can be loaded again")
	}
}

func (pm *defaultPluginManager) IsPluginLoaded(key string) bool {
	pm.pluginsMux.RLock()
	_, exists := pm.plugins[key]
	pm.pluginsMux.RUnlock()

	return exists
}

func (pm *defaultPluginManager) WasPreviouslyLoaded(key string) (bool, map[string]string) {
	pm.previouslyLoadedMux.RLock()
	wasLoaded, exists := pm.previouslyLoaded[key]
	pm.previouslyLoadedMux.RUnlock()

	// Get the last known config for this function
	var config map[string]string
	pm.pluginConfigsMux.RLock()
	lastConfig, hasConfig := pm.pluginConfigs[key]
	if hasConfig {
		// Make a copy of the config
		config = make(map[string]string, len(lastConfig))
		for k, v := range lastConfig {
			config[k] = v
		}
	}
	pm.pluginConfigsMux.RUnlock()

	return exists && wasLoaded, config
}

// HasConfigChanged compares stored config with a new config to check for changes.
func (pm *defaultPluginManager) HasConfigChanged(key string, newConfig map[string]string) bool {
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

func (pm *defaultPluginManager) HasDigestChanged(key string, newDigest string) bool {
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

func (pm *defaultPluginManager) GetLogStore() *logging.FunctionLogStore {
	return pm.logStore
}

func (pm *defaultPluginManager) GetPluginDigest(key string) (string, bool) {
	pm.pluginDigestsMux.RLock()
	digest, exists := pm.pluginDigests[key]
	pm.pluginDigestsMux.RUnlock()

	return digest, exists
}

func (pm *defaultPluginManager) GetPluginConfig(key string) (map[string]string, bool) {
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

// GetPreviouslyLoadedFunctions returns a map of all functions that have been previously loaded.
func (pm *defaultPluginManager) GetPreviouslyLoadedFunctions() map[string]bool {
	pm.previouslyLoadedMux.RLock()
	defer pm.previouslyLoadedMux.RUnlock()

	// Make a copy to avoid concurrency issues
	result := make(map[string]bool, len(pm.previouslyLoaded))
	for k, v := range pm.previouslyLoaded {
		result[k] = v
	}

	return result
}

// GetStoppedFunctions returns a map of all functions that have been stopped.
func (pm *defaultPluginManager) GetStoppedFunctions() map[string]bool {
	pm.stoppedFunctionsMux.RLock()
	defer pm.stoppedFunctionsMux.RUnlock()

	// Make a copy to avoid concurrency issues
	result := make(map[string]bool, len(pm.stoppedFunctions))
	for k, v := range pm.stoppedFunctions {
		result[k] = v
	}

	return result
}

func (pm *defaultPluginManager) Shutdown() {
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

// ListLoadedFunctions returns a list of currently loaded function keys.
func (pm *defaultPluginManager) ListLoadedFunctions() []string {
	pm.pluginsMux.RLock()
	defer pm.pluginsMux.RUnlock()

	keys := make([]string, 0, len(pm.plugins))
	for key := range pm.plugins {
		keys = append(keys, key)
	}

	return keys
}

// GetLoadedFunctionCount returns the number of currently loaded functions.
func (pm *defaultPluginManager) GetLoadedFunctionCount() int {
	pm.pluginsMux.RLock()
	defer pm.pluginsMux.RUnlock()

	return len(pm.plugins)
}
