package di

import (
	"fmt"
	"path/filepath"

	"github.com/dgraph-io/badger/v4"
	"github.com/ignitionstack/ignition/internal/repository"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/engine"
	"github.com/ignitionstack/ignition/pkg/registry"
	localRegistry "github.com/ignitionstack/ignition/pkg/registry/local"
)

// Container manages dependency injection for the application
type Container struct {
	functionService services.FunctionService
}

// NewContainer creates a new DI container
func NewContainer() *Container {
	return &Container{
		functionService: services.NewFunctionService(),
	}
}

// CreateEngine creates a fully configured engine instance
func (c *Container) CreateEngine(socketPath, httpAddr, registryDir string, logger engine.Logger) (*engine.Engine, error) {
	// Create registry
	registry, err := c.createRegistry(registryDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create registry: %w", err)
	}

	// Create engine with all dependencies
	return engine.NewEngineWithDependencies(
		socketPath,
		httpAddr,
		registry,
		c.functionService,
		logger,
	), nil
}

// createRegistry creates and initializes a registry instance
func (c *Container) createRegistry(registryDir string) (registry.Registry, error) {
	// Setup database
	opts := badger.DefaultOptions(filepath.Join(registryDir, "registry.db"))
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open registry database: %w", err)
	}

	// Create repository
	dbRepo := repository.NewBadgerDBRepository(db)

	// Create registry
	return localRegistry.NewLocalRegistry(registryDir, dbRepo), nil
}