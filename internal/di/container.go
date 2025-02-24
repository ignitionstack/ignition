package di

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/dgraph-io/badger/v4"
	"github.com/ignitionstack/ignition/internal/repository"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/engine"
	"github.com/ignitionstack/ignition/pkg/registry"
	localRegistry "github.com/ignitionstack/ignition/pkg/registry/local"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

// Module exports all the DI providers
var Module = fx.Options(
	fx.Provide(
		// Base zap logger
		NewZapBaseLogger,
		
		// DB and repositories
		NewBadgerDB,
		NewDBRepository,
		NewRegistry,
		
		// Services
		services.NewFunctionService,
		NewEngine,
		
		// Engine logger
		fx.Annotate(
			NewEngineLogger,
			fx.As(new(engine.Logger)),
		),
	),
	
	// Add logging of providers and lifecycle events
	fx.WithLogger(func(logger *zap.Logger) fxevent.Logger {
		return &fxevent.ZapLogger{Logger: logger}
	}),
)

// AppConfig holds configuration for the application
type AppConfig struct {
	SocketPath  string
	HTTPAddr    string
	RegistryDir string
}

// NewAppConfig creates a new application configuration
func NewAppConfig(socketPath, httpAddr, registryDir string) AppConfig {
	return AppConfig{
		SocketPath:  socketPath,
		HTTPAddr:    httpAddr,
		RegistryDir: registryDir,
	}
}

// EngineParams contains all dependencies needed to create an Engine
type EngineParams struct {
	fx.In

	Registry        registry.Registry
	FunctionService services.FunctionService
	Config          AppConfig
	Logger          engine.Logger
}

// NewZapBaseLogger creates the base zap logger that will be used by fx
func NewZapBaseLogger(lc fx.Lifecycle) (*zap.Logger, error) {
	// For development, use a more readable logger configuration
	config := zap.NewDevelopmentConfig()
	
	zapLogger, err := config.Build()
	if err != nil {
		return nil, err
	}

	// Register cleanup
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return zapLogger.Sync()
		},
	})

	return zapLogger, nil
}

// NewEngineLogger creates an engine.Logger adapter using the base zap logger
func NewEngineLogger(baseLogger *zap.Logger) engine.Logger {
	return &zapLoggerAdapter{baseLogger.Sugar()}
}

type zapLoggerAdapter struct {
	logger *zap.SugaredLogger
}

func (z *zapLoggerAdapter) Printf(format string, args ...interface{}) {
	z.logger.Infof(format, args...)
}

func (z *zapLoggerAdapter) Errorf(format string, args ...interface{}) {
	z.logger.Errorf(format, args...)
}

func (z *zapLoggerAdapter) Debugf(format string, args ...interface{}) {
	z.logger.Debugf(format, args...)
}

// NewBadgerDB creates a new BadgerDB instance
func NewBadgerDB(lc fx.Lifecycle, config AppConfig) (*badger.DB, error) {
	opts := badger.DefaultOptions(filepath.Join(config.RegistryDir, "registry.db"))
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open registry database: %w", err)
	}

	// Register lifecycle hooks for proper cleanup
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return db.Close()
		},
	})

	return db, nil
}

// NewDBRepository creates a new DB repository
func NewDBRepository(db *badger.DB) repository.DBRepository {
	return repository.NewBadgerDBRepository(db)
}

// NewRegistry creates a new registry
func NewRegistry(dbRepo repository.DBRepository, config AppConfig) registry.Registry {
	return localRegistry.NewLocalRegistry(config.RegistryDir, dbRepo)
}

// NewEngine creates a new engine
func NewEngine(params EngineParams) *engine.Engine {
	return engine.NewEngineWithDependencies(
		params.Config.SocketPath,
		params.Config.HTTPAddr,
		params.Registry,
		params.FunctionService,
		params.Logger,
	)
}