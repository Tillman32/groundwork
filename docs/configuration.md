# Configuration

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | No | `sqlite:./data.db` | Database connection string |
| `JWT_SECRET` | Yes | - | Secret for JWT token signing |
| `SERVER_PORT` | No | `8080` | HTTP server port |
| `WS_PORT` | No | `8081` | WebSocket server port |
| `LOG_LEVEL` | No | `info` | Log level (debug, info, warn, error) |

## Database

### SQLite (development)
```bash
# Default - no config needed
./groundworkd
```

### PostgreSQL (production)
```bash
DATABASE_URL=postgres://user:password@localhost/groundwork?sslmode=disable ./groundworkd
```

## JWT Secret

Generate a secure secret:
```bash
# Generate
openssl rand -base64 32

# Or use
head -c 32 /dev/urandom | base64
```

## Example

```bash
export JWT_SECRET="your-32-byte-secret-here"
export DATABASE_URL="sqlite:/data/groundwork.db"
export SERVER_PORT=443
export WS_PORT=443

./groundworkd
```