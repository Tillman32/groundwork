# Installation

## Requirements

- Go 1.22+ (for building from source)
- SQLite or PostgreSQL (for database)

## Quick Install

```bash
# Download latest release
curl -L https://github.com/Tillman32/groundwork/releases/latest/download/gw-agent-linux-amd64 -o gw-agent
chmod +x gw-agent

# Or build from source
git clone https://github.com/Tillman32/groundwork
cd groundwork
go build -o groundworkd ./cmd/groundworkd
go build -o gw-agent ./cmd/gw-agent
```

## Binary Releases

| Architecture | groundworkd | gw-agent |
|--------------|-------------|----------|
| Linux AMD64 | groundworkd-linux-amd64 | gw-agent-linux-amd64 |
| Linux ARM64 | groundworkd-linux-arm64 | gw-agent-linux-arm64 |

Download from [Releases](https://github.com/Tillman32/groundwork/releases).

## Running

### Server

```bash
# With defaults (sqlite database)
./groundworkd

# With custom config
DATABASE_URL=postgres://user:pass@localhost/groundwork JWT_SECRET=your-secret ./groundworkd
```

### Agent

```bash
# Basic connection
./gw-agent --server ws://localhost:8081/ws --token <enrollment-token>
```