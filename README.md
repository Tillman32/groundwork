# Groundwork

Agent/server system with plugin management, authentication, policy, and trust components.

## Overview

Groundwork is a Go-based infrastructure for managing autonomous agents with:
- **Agent**: WebSocket-based endpoint agents with heartbeat and job execution
- **Server**: REST API with Gin for control plane operations
- **Plugin System**: Go and PowerShell plugin runtimes with verification
- **Registry**: Plugin discovery and caching
- **Notify**: Notification management (email via jordan-wright/email)
- **Trust**: Trust score management for agents/plugins
- **Policy**: Authorization policies

## Quick Start

### Prerequisites
- Go 1.22+
- SQLite (or PostgreSQL for production)

### Build
```bash
git clone https://github.com/Tillman32/groundwork
cd groundwork
go mod download
go build ./...
```

### Run Server
```bash
# Build
go build -o groundworkd ./cmd/groundworkd

# Run (default: sqlite database at ./data.db)
./groundworkd
```

### Run Agent
```bash
# Build
go build -o gw-agent ./cmd/gw-agent

# Run (connects to server via WebSocket)
./gw-agent --server ws://localhost:8080/ws --token <enrollment-token>
```

## Architecture

```
┌─────────────────┐         ┌─────────────────┐
│   gw-agent      │◀──────▶│  groundworkd    │
│  (endpoint)     │  WS     │  (control      │
└─────────────────┘         │   plane)        │
                            └─────────────────┘
                                   │
                    ┌──────────────┼──────────────┐
                    ▼              ▼              ▼
               ┌─────────┐   ┌──────────┐   ┌──────────┐
               │plugins  │   │registry │   │notify    │
               └─────────┘   └──────────┘   └──────────┘
```

## Configuration

Environment variables:
- `DATABASE_URL` - Database connection (default: `sqlite:./data.db`)
- `JWT_SECRET` - Secret for JWT signing (required for auth)
- `SERVER_PORT` - HTTP server port (default: `8080`)
- `WS_PORT` - WebSocket port (default: `8081`)

## Development

### Project Structure
```
groundwork/
├── cmd/
│   ├── groundworkd/     # Server entry point
│   └── gw-agent/        # Agent entry point
├── internal/
│   ├── agent/           # Agent implementation
│   ├── notify/          # Notification manager
│   ├── registry/        # Plugin registry
│   ├── server/
│   │   ├── api/         # REST endpoints
│   │   ├── auth/        # Authentication
│   │   ├── backup/      # Backup manager
│   │   ├── policy/      # Policy engine
│   │   ├── trust/       # Trust scores
│   │   ├── transport/   # WebSocket hub
│   │   ├── update/      # Update checker
│   │   └── web/         # Embedded UI
│   ├── plugin/          # Plugin system
│   └── store/           # Data layer
└── go.mod
```

### Adding a Plugin

1. Create Go binary in `cmd/plugins/`
2. Build to `plugins/` directory
3. Register via API or registry sync

## Documentation

- [Installation](docs/installation.md)
- [Configuration](docs/configuration.md)
- [Plugins](docs/plugins.md)
- [API Reference](docs/api.md)
- [Contributing](docs/contributing.md)

## License

MIT