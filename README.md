# Ignition

Ignition is a powerful CLI tool for building, managing, and running WebAssembly functions using Extism. Think of it as "Docker for WebAssembly functions" - it provides a familiar workflow for developers to build, share, and execute WebAssembly modules.

<img src=".github/assets/ignition.gif" alt="ignition demo" width="600" />

## Features
- Build WebAssembly functions from multiple languages (Rust, TypeScript, JavaScript, Go)
- Local registry for storing and managing your functions
- Function versioning and tagging
- Simple function execution interface
- HTTP API for function invocation
- Built-in support for Extism PDK

## Installation
```bash
# From source
git clone https://github.com/ignitionstack/ignition
cd ignition
go build
```

## Quick Start

### Start the engine

```bash
ignition engine start
```

### Initialize a New Function
```bash
# Create a new function (you'll be prompted to select a language)
ignition init my_function
```

### Build a Function
```bash
# Build with namespace and tag
ignition build -t my_namespace/my_function:latest my_function/
```

### List Functions
```bash
# List functions in a namespace
ignition function ls my_namespace/my_function
```

### Executing Functions

There are two ways to execute functions:

1. Direct Invocation:
```bash
# Call the function directly via CLI. Entrypoints defaults to 'handler'
ignition call my_namespace/my_function -e greet -p "ignition"
```

2. HTTP API:
First, load the function into the engine:
```bash
# Start the function in the engine (required for HTTP API access)
ignition run my_namespace/my_function:latest
```

Then call it via HTTP:
```bash
# Call function via HTTP API
http POST http://localhost:8080/my_namespace/my_function/greet payload=ignition
```

Note: The `run` command is only needed when you want to access the function through the HTTP API. For direct CLI invocation using `call`, you don't need to run the function first.

## Function Development
Ignition functions are built using [Extism Plugin Development Kits (PDKs)](https://extism.org/docs/concepts/pdk/). Extism PDKs provide the foundation for writing WebAssembly plugins in your preferred programming language. Check out the Extism PDK documentation to learn about the programming model and available features.

### Configuration
Functions are configured using `ignition.yml`:
```yaml
function:
  name: my_function
  language: rust
  settings:
    enable_wasi: true
    allowed_urls:
      - "https://api.example.com"
```

### Supported Languages
Ignition supports multiple languages for writing functions:
- Rust (using Extism Rust PDK)
- TypeScript (using Extism TS PDK)
- JavaScript (using Extism JS PDK)
- Go (using Extism Go PDK)

Each language template comes with:
- Proper project structure
- Development environment setup
- Basic function implementation
- Test examples

## Registry
Ignition includes a local registry for managing your functions. Functions are identified by:
```
namespace/name:tag
```

For example:
```
my_namespace/my_function:latest
```

### Tags and Versions
Functions can be tagged during build:
```bash
# Build with a specific tag
ignition build -t my_namespace/my_function:v1.0.0 my_function/
```

## HTTP API
Ignition provides an HTTP API for function invocation. Functions are accessible at:
```
http://localhost:8080/{namespace}/{function}/{endpoint}
```

Example:
```bash
# Call a function's greet endpoint
http POST http://localhost:8080/my_namespace/my_function/greet payload=ignition
```

## Development Status
Ignition is currently under heavy development. The API and features are subject to change, and we encourage you to try it out and provide feedback!

## Contributing
We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## License
MIT License - see [LICENSE](LICENSE) for details.

## Similar Projects
- [Extism](https://extism.org/) - The Extensible Plugin System
- [WASI](https://wasi.dev/) - The WebAssembly System Interface
- [Spin](https://developer.fermyon.com/spin/index) - A framework for building and running WebAssembly microservices

## Support
- [GitHub Issues](https://github.com/ignitionstack/ignition/issues)
- [Discord Community](https://discord.gg/EQeZY5utHC)
