package di

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/ignitionstack/ignition/internal/repository"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/engine"
	"github.com/ignitionstack/ignition/pkg/engine/config"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/ignitionstack/ignition/pkg/registry"
	localRegistry "github.com/ignitionstack/ignition/pkg/registry/local"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Container struct {
	services map[string]interface{}
	mu       sync.RWMutex
}

func NewContainer() *Container {
	return &Container{
		services: make(map[string]interface{}),
	}
}

func (c *Container) Register(name string, service interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.services[name] = service
}

func (c *Container) Get(name string) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	service, ok := c.services[name]
	if !ok {
		return nil, fmt.Errorf("service not found: %s", name)
	}

	return service, nil
}

type QuietLogger struct {
	Logger *zap.Logger
}

func (l *QuietLogger) LogEvent(event fxevent.Event) {
	switch e := event.(type) {
	case *fxevent.Started:
		// Log application startup
		l.Logger.Info("Application started")
	case *fxevent.Stopping:
		// Log application shutdown
		l.Logger.Info("Application stopping")
	case *fxevent.Stopped:
		// Log application shutdown status
		if e.Err != nil {
			l.Logger.Error("Application stopped with error", zap.Error(e.Err))
		} else {
			l.Logger.Info("Application stopped gracefully")
		}
	case *fxevent.Invoked:
		// Only log errors when invoking functions
		if e.Err != nil {
			l.Logger.Error("Error invoking function",
				zap.String("function", e.FunctionName),
				zap.Error(e.Err))
		}
	case *fxevent.Provided:
		// Skip all provided logs - too verbose
	case *fxevent.Supplied:
		// Skip all supplied logs - too verbose
	default:
		// Only log critical errors from other event types
		// For most event types we don't need to log anything, making output cleaner
	}
}

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
			fx.As(new(logging.Logger)),
		),
	),

	// Use our custom quiet logger
	fx.WithLogger(func(logger *zap.Logger) fxevent.Logger {
		return &QuietLogger{Logger: logger}
	}),

	// Configure fx to be less verbose
	fx.NopLogger,
)

type AppConfig struct {
	SocketPath  string
	HTTPAddr    string
	RegistryDir string
}

type EngineParams struct {
	fx.In

	Registry        registry.Registry
	FunctionService services.FunctionService
	Config          AppConfig
	Logger          logging.Logger
}

func NewZapBaseLogger(lc fx.Lifecycle) (*zap.Logger, error) {
	// Create a custom, quieter configuration
	config := zap.NewProductionConfig()

	// Adjust log level to only show warnings and above by default
	config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)

	// Make the output more concise
	config.DisableStacktrace = true
	config.DisableCaller = true
	config.Encoding = "console"

	// Simplify the output format
	config.EncoderConfig.TimeKey = "time"
	config.EncoderConfig.LevelKey = "level"
	config.EncoderConfig.MessageKey = "msg"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	zapLogger, err := config.Build()
	if err != nil {
		return nil, err
	}

	// Register cleanup
	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return zapLogger.Sync()
		},
	})

	return zapLogger, nil
}

func NewEngineLogger(_ *zap.Logger) logging.Logger {
	// Create a specialized logger for engine with higher verbosity
	config := zap.NewDevelopmentConfig()

	// Only show important information by default
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)

	// Simplify the output - we don't need timestamps or log levels in server logs
	config.Encoding = "console"
	config.EncoderConfig.TimeKey = ""     // No timestamps
	config.EncoderConfig.LevelKey = ""    // No log level
	config.EncoderConfig.EncodeTime = nil // No time encoder needed
	config.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	config.DisableCaller = true
	config.DisableStacktrace = true

	engineLogger, _ := config.Build()

	return &zapLoggerAdapter{engineLogger.Sugar()}
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

func NewBadgerDB(lc fx.Lifecycle, config AppConfig) (*badger.DB, error) {
	opts := badger.DefaultOptions(filepath.Join(config.RegistryDir, "registry.db"))
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open registry database: %w", err)
	}

	// Register lifecycle hooks for proper cleanup
	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return db.Close()
		},
	})

	return db, nil
}

func NewDBRepository(db *badger.DB) repository.DBRepository {
	return repository.NewBadgerDBRepository(db)
}

func NewRegistry(dbRepo repository.DBRepository, config AppConfig) registry.Registry {
	return localRegistry.NewLocalRegistry(config.RegistryDir, dbRepo)
}

func NewEngine(params EngineParams) *engine.Engine {
	// Create config object
	cfg := &config.Config{
		Engine: config.EngineConfig{
			DefaultTimeout:   30 * time.Second,
			LogStoreCapacity: 1000,
			CircuitBreaker: config.CircuitBreakerConfig{
				FailureThreshold: 5,
				ResetTimeout:     30 * time.Second,
			},
			PluginManager: config.PluginManagerConfig{
				TTL:             10 * time.Minute,
				CleanupInterval: 1 * time.Minute,
			},
		},
		Server: config.ServerConfig{
			SocketPath:  params.Config.SocketPath,
			HTTPAddr:    params.Config.HTTPAddr,
			RegistryDir: params.Config.RegistryDir,
		},
	}

	// Create engine with the config
	engine, err := engine.NewEngineWithConfig(cfg, params.Logger)
	if err != nil {
		// In a production system, we'd handle this error better
		// but in this DI context, panicking is acceptable
		panic("failed to create engine: " + err.Error())
	}

	return engine
}
