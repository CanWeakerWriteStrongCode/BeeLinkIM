# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
# Build the server binary
go build -o chat-server ./cmd/chat-server

# Run (requires configs/config.yaml)
./chat-server

# Run all tests
go test ./...

# Tidy dependencies
go mod tidy

# Full stack via Docker (MySQL, Redis, RocketMQ, app)
docker-compose up
```

## Architecture

BeeLinkIM is a **distributed real-time chat server** in Go, designed for horizontal scaling. It follows a layered architecture with manual constructor-based dependency injection.

**Tech stack:** Gin (HTTP), gorilla/websocket (WebSocket), GORM + MySQL (persistence), go-redis + redislock (caching/sessions/locks/routing), Apache RocketMQ (cross-server messaging), Viper (config), Zap + lumberjack (logging), golang-jwt (HS256 auth).

### Directory Layout

| Path | Purpose |
|---|---|
| `cmd/chat-server/main.go` | Entry point: wires all dependencies, starts HTTP server with graceful shutdown |
| `configs/config.yaml` | Single YAML config (server, MySQL, Redis, RocketMQ, app) |
| `internal/handler/` | Gin HTTP handlers (health, online count, search, upload, login) |
| `internal/middleware/` | CORS, JWT auth (supports `Authorization: Bearer` header and `?token=` query param for WS), atomic online counter |
| `internal/router/router.go` | Route registration + `GlobalResp` wrapper for unified JSON responses |
| `internal/api/ws_handler.go` | WebSocket upgrader (gorilla/websocket) |
| `internal/service/chat_service.go` | Core business logic: WS lifecycle, message send flow, MQ consumer startup |
| `internal/ws/` | `Hub` (channel-based connection manager with RWMutex) and `Client` (per-user WS with read/write pump goroutines) |
| `internal/mq/rocketmq.go` | RocketMQ producer manager |
| `internal/repository/mysqlx/` | GORM models (`TChatRoom`, `TChatMessage`) + repo with AutoMigrate |
| `internal/repository/redisx/` | Redis repo: room sessions (Hash), user-server routing (24h TTL), atomic sequences (`HIncrBy`), distributed locks |
| `internal/dto/` | Response DTO |
| `pkg/config/` | Viper YAML loader, `GlobalConfig` singleton |
| `pkg/errorx/` | Custom `BizError` type + error codes (0=success, 1xxx=system, 2xxx=chat business) |
| `pkg/jwt/` | JWT generation/parsing (HS256, 7-day expiry, issuer "BeeLinkIM") |
| `pkg/logger/` | Zap dual-output (JSON→file with rotation, console→stdout) + `Info/Warn/Error/Fatal` shortcuts |

### Key Patterns

1. **Constructor DI**: All dependencies injected via `NewXxx` constructors, wired in `main.go`. No DI framework.

2. **Hub + Client (WebSocket)**: `Hub` manages `uid → *Client` map via `Register`/`Unregister` channels (serialized in `Run()` select loop). Each `Client` has independent `ReadPump`/`WritePump` goroutines with Ping/Pong heartbeat (54s/60s intervals). Send channel buffer is 256.

3. **Distributed Message Routing** (`service/chat_service.go:SendMessage`): Check if recipient is locally connected via `hub.FindByUid` → push directly via Hub; otherwise → publish to RocketMQ → target server's push consumer delivers to local client.

4. **Room Session Model**: 1-on-1 chat rooms identified by `(minUID, maxUID)` pair. Stored in MySQL (source of truth) + Redis Hash cache (`room:session:{uid1}-{uid2}` with `room_id` + `sequence` fields).

5. **Sequence-based Ordering**: Redis `HIncrBy` provides atomic per-room sequence numbers for message ordering and deduplication.

6. **Double-Checked Locking**: Room creation: Redis cache check → distributed lock → double-check → MySQL `GetOrCreateRoom` → cache session.

7. **Graceful Shutdown**: `main.go` listens for SIGINT/SIGTERM, then calls `srv.Shutdown(ctx)` with a 5-second timeout.

### Startup Sequence (main.go)

```
Logger → Config → MySQL (GORM + AutoMigrate) → Redis → Distributed Lock
→ Repositories → WebSocket Hub (goroutine) → RocketMQ Producer
→ WS Handler → ChatService → MQ Consumer → Router → HTTP Server (goroutine)
→ Wait for shutdown signal → Graceful shutdown
```
