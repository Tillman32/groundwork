# API

## Endpoints

### Auth

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/auth/register` | Register new user |
| POST | `/api/v1/auth/login` | Login, get JWT |
| POST | `/api/v1/auth/refresh` | Refresh JWT |

### Agents

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/agents` | List all agents |
| GET | `/api/v1/agents/{id}` | Get agent details |
| POST | `/api/v1/agents/{id}/ping` | Ping agent |

### Plugins

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/plugins` | List plugins |
| POST | `/api/v1/plugins/install` | Install plugin |
| POST | `/api/v1/registry/sync` | Sync plugin registry |

### Backup

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/backup` | Create backup |
| GET | `/api/v1/backup` | List backups |
| GET | `/api/v1/backup/{id}/download` | Download backup |

### Trust

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/trust` | List trust scores |
| POST | `/api/v1/trust/{agent}/score` | Add trust score |

### Policy

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/policies` | List policies |
| POST | `/api/v1/policies` | Create policy |

## WebSocket

Agent connection at `ws://server:8081/ws`:
```bash
# Connect
wscat -c ws://localhost:8081/ws -H "Authorization: Bearer <token>"

# Heartbeat (sent automatically)
{"type": "heartbeat", "data": {}}
```