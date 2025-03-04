# Ignition

*Ignition is still in development, things can and will change*

Ignition is a powerful CLI tool for building, managing, and running WebAssembly functions using [Extism](https://extism.org/). Think of it as "Docker for WebAssembly functions" - providing a familiar workflow for developers to build, share, and execute WebAssembly modules without the complexity.

<img src=".github/assets/ignition.gif" alt="ignition demo" width="600" />

## Why Ignition?

- **Language-agnostic**: Build WebAssembly functions in Rust, TypeScript, JavaScript, or Go
- **Simple workflow**: Familiar commands for building, running, and managing functions
- **Local registry**: Store and version your functions locally
- **Dual execution modes**: Run functions via CLI or HTTP API
- **Compose support**: Define and run multi-function applications with compose files
- **Built on standards**: Uses Extism PDK for WebAssembly development

## Installation

```bash
# Install from source
git clone https://github.com/ignitionstack/ignition
cd ignition
go build
```

## Quick Start Guide

### 1. Start the Ignition Engine

The engine powers the HTTP API and function execution environment:

```bash
ignition engine start
```

The engine can be configured using a YAML file, environment variables, or command-line flags:

```bash
# Start with a custom configuration file
ignition engine start --config /path/to/config.yaml

# Show the current configuration
ignition engine start --show-config

# Override specific settings
ignition engine start --socket /tmp/custom-socket.sock --http :9090
```

#### Engine Configuration

Ignition uses a flexible configuration system based on:

1. **Configuration File**: Default location is `~/.ignition/config.yaml`
2. **Environment Variables**: Use `IGNITION_` prefix (e.g., `IGNITION_ENGINE_DEFAULT_TIMEOUT=60s`)
3. **Command-line Flags**: Take highest precedence

Example configuration file:

```yaml
# Server configuration
server:
  socket_path: ~/.ignition/engine.sock
  http_addr: :8080
  registry_dir: ~/.ignition/registry

# Engine configuration
engine:
  default_timeout: 30s
  log_store_capacity: 1000
  
  circuit_breaker:
    failure_threshold: 5
    reset_timeout: 30s
  
  plugin_manager:
    ttl: 10m
    cleanup_interval: 1m
```

See the [example-config.yaml](example-config.yaml) file for a complete configuration template.

### 2. Create a New Function

```bash
# Creates a new function project (interactive language selection)
ignition init my_function
```

### 3. Build Your Function

```bash
# Build with namespace/name:tag format
ignition build -t my_namespace/my_function:latest my_function/
```

### 4. Execute Your Function

**Method 1: Direct CLI Invocation**
```bash
# Call with optional entrypoint (-e), payload (-p), and config (-c)
ignition call my_namespace/my_function -e greet -p "ignition" -c key=value -c another=value
```

**Method 2: HTTP API**
```bash
# First, load the function into the engine (optionally with config)
ignition run my_namespace/my_function:latest -c key=value -c another=value

# Then call via HTTP (requires httpie or similar tool)
http POST http://localhost:8080/my_namespace/my_function/greet payload=ignition
```

> **Note:** The `run` command is only needed for HTTP API access. CLI invocation with `call` works without it.

## Function Development

### Configuration (ignition.yml)

Every function requires an `ignition.yml` configuration file:

```yaml
function:
  name: my_function
  language: rust  # rust, typescript, javascript, or go
  settings:
    enable_wasi: true  # Enable WASI capabilities
    allowed_urls:      # External URLs the function can access
      - "https://api.example.com"
```

### Function Configuration

You can pass configuration values to functions at runtime:

```bash
# Using the run command
ignition run my_namespace/my_function -c vowels=aeiou -c debug=true

# Using the call command
ignition call my_namespace/my_function -e count_vowels -p "Hello" -c vowels=aeiou

# Using compose (in ignition-compose.yml)
services:
  my_service:
    function: my_namespace/my_function:latest
    config:
      vowels: "aeiou"
      debug: "true"
```

These values are accessible within your function via the Extism plugin config mechanism.

### Supported Languages

Ignition provides templates for multiple languages:

| Language       | Based On          |
|----------------|-------------------|
| Rust           | Extism Rust PDK   |
| TypeScript     | Extism TS PDK     |
| JavaScript     | Extism JS PDK     |
| Go             | Extism Go PDK     |
| Python         | Extism Python PDK |
| AssemblyScript | Extism AssemblyScript PDK    |

Each template includes project structure, dependencies, and example code.

## Managing Functions

### List Functions

```bash
# List all functions in a namespace
ignition function ls my_namespace/my_function

# List all running functions
ignition ps
```

### Versioning with Tags

Functions use Docker-like tagging for versioning:

```bash
# Build with specific tag
ignition build -t my_namespace/my_function:v1.0.0 my_function/
```

## Using Compose

*WARNING: This feature is still in active development so some things might not work or not be implemented yet*

Ignition supports a Docker Compose-like workflow for running multiple functions together.

### Initialize a Compose File

```bash
# Create a new ignition-compose.yml file
ignition compose init
```

### Define Your Services

Edit the generated `ignition-compose.yml` file:

```yaml
version: "1"
services:
  api:
    function: my_namespace/api_service:latest
    config:
      debug: "true"
      greeting: "hallo"
  processor:
    function: my_namespace/processor:v1.2.0
    depends_on:
      - api
  worker:
    function: my_namespace/worker:latest
```

### Start Services

```bash
# Load and run all functions defined in ignition-compose.yml
ignition compose up

# Use a different compose file
ignition compose up -f my-compose.yml

# Run in detached mode
ignition compose up -d
```

### Check Running Functions

```bash
# List all running functions
ignition ps
```

### Stop Services

```bash
# Stop all functions defined in ignition-compose.yml
ignition compose down
```

## HTTP API Reference

When functions are loaded with `ignition run`, they're accessible via HTTP:

```
http://localhost:8080/{namespace}/{function}/{endpoint}
```

**Example Request:**
```bash
http POST http://localhost:8080/my_namespace/my_function/greet payload=ignition
```

## Development Status

Ignition is under active development. APIs and features may change. We welcome your feedback and contributions!

## Resources

- [Extism Documentation](https://extism.org/docs/concepts/pdk/) - Learn about the WebAssembly PDK
- [GitHub Issues](https://github.com/ignitionstack/ignition/issues) - Report bugs or request features

## Similar Projects

- [Extism](https://extism.org/) - The Extensible Plugin System
- [WASI](https://wasi.dev/) - The WebAssembly System Interface
- [Spin](https://developer.fermyon.com/spin/index) - WebAssembly microservices framework

## License

MIT License - see [LICENSE](LICENSE) for details.
