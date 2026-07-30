# stashkit

> Agentic-first key-value store service. Plain text API, agent-driven, single Go binary.

stashkit is a simple, fast key-value store designed for AI agents to persist and retrieve arbitrary data. No UI, no SDK — the agent IS the interface. The API is the product.

## Quick Start

```bash
# Build and run
make build && ./stashkit

# Or with Go directly
CGO_ENABLED=0 go build -trimpath -o stashkit ./cmd/stashkit
./stashkit

# It listens on :7788 by default
```

## Auth Flow

```bash
# 1. Request OTP
curl -X POST http://localhost:7788/auth/request -d "email=agent@example.com"
# → ok: OTP generated (check server logs for code in dev mode)

# 2. Verify OTP → get bearer token
curl -X POST http://localhost:7788/auth/verify -d "email=agent@example.com" -d "code=123456"
# → token=eyJhbGciOi... email=agent@example.com

# 3. Use token for all requests
curl -H "Authorization: Bearer <token>" http://localhost:7788/entries
```

## API Reference

### Store a value
```bash
curl -X POST http://localhost:7788/entries \
  -H "Authorization: Bearer <token>" \
  -d "key=api_key" -d "value=sk-12345" -d "namespace=secrets"
# → handle=stash_a1b2c key=api_key value=sk-12345 namespace=secrets created=2026-07-30T01:30:00Z updated=2026-07-30T01:30:00Z
```

### Get value by key
```bash
curl "http://localhost:7788/lookup?key=api_key&namespace=secrets" \
  -H "Authorization: Bearer <token>"
# → handle=stash_a1b2c key=api_key value=sk-12345 namespace=secrets updated=2026-07-30T01:30:00Z
```

### List all entries
```bash
curl http://localhost:7788/entries \
  -H "Authorization: Bearer <token>"
# → handle=stash_a1b2c key=api_key value=sk-12345 namespace=secrets updated=...
# → handle=stash_x9y8z key=config value=prod namespace= updated=...
```

### Set with TTL (auto-expire)
```bash
curl -X POST http://localhost:7788/entries \
  -H "Authorization: Bearer <token>" \
  -d "key=session" -d "value=abc123" -d "ttl=3600"
# → handle=stash_b2c3d key=session value=abc123 namespace= ... expires=2026-07-30T02:30:00Z
```

### Delete by key
```bash
curl -X DELETE "http://localhost:7788/lookup?key=api_key&namespace=secrets" \
  -H "Authorization: Bearer <token>"
# → ok: entry deleted
```

### Workspaces
```bash
# Create
curl -X POST http://localhost:7788/workspaces \
  -H "Authorization: Bearer <token>" \
  -d "name=production"
# → handle=ws_abc12 name=production plan=free

# List
curl http://localhost:7788/workspaces \
  -H "Authorization: Bearer <token>"
# → handle=ws_abc12 name=production plan=free entries=5

# Use specific workspace
curl "http://localhost:7788/entries?ws=ws_abc12" \
  -H "Authorization: Bearer <token>"
```

### Audit Log
```bash
curl http://localhost:7788/audit \
  -H "Authorization: Bearer <token>"
# → id=1 action=entry.put detail=key=api_key namespace=secrets actor=agent@example.com time=...
```

### JSON Format
Add `Accept: application/json` header or `?format=json` query param:
```bash
curl "http://localhost:7788/entries?format=json" \
  -H "Authorization: Bearer <token>"
```

### Self-Documentation
```bash
curl http://localhost:7788/help
# Returns full operating manual for agents
```

## Configuration

| Setting | Flag | Env Var | Default |
|---------|------|---------|---------|
| Listen address | `-addr` | `STASHKIT_ADDR` | `:7788` |
| Data directory | `-data` | `STASHKIT_DATA` | `./data` |
| Token secret | `-secret` | `STASHKIT_SECRET` | auto-generated |
| SMTP host | `-smtp-host` | `STASHKIT_SMTP_HOST` | (none) |
| SMTP port | `-smtp-port` | `STASHKIT_SMTP_PORT` | (none) |
| SMTP user | `-smtp-user` | `STASHKIT_SMTP_USER` | (none) |
| SMTP pass | `-smtp-pass` | `STASHKIT_SMTP_PASS` | (none) |
| SMTP from | `-smtp-from` | `STASHKIT_SMTP_FROM` | (none) |

Config priority: defaults < env vars < flags

## Build

```bash
make build    # CGO_ENABLED=0, static binary
make test     # go test -race
make vet      # go vet
make run      # build + run
make clean    # remove binaries
```

## Architecture

- **Single binary** — Go, zero external dependencies, CGO_ENABLED=0
- **JSON file storage** — Simple, portable, no database needed
- **Multi-tenant** — Workspaces isolate data per tenant
- **Audit log** — All mutations are logged
- **TTL support** — Entries can auto-expire
- **Namespaces** — Logical grouping within a workspace
- **MCP endpoint** — `/mcp` for Model Context Protocol integrations

## License

MIT
