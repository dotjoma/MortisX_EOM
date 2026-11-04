# Madrigal Multibot — Architecture Specification

Production-grade, multi-instance bot manager architecture built on Go 1.22+, Wails v2, and React 18 / TypeScript.

---

## 1. Architectural Layers & Inward Dependency Rule

Dependencies strictly point **inward**. Inner layers never reference outer layers.

```
+-------------------------------------------------------------+
|  UI Layer (React 18 + TS Components)                        |
|  - Dumb presentational components (props in, events out)     |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
|  Store & API Layer (src/lib/stores/, src/lib/api/)           |
|  - Sole callers of Wails bindings (window.go.main.App)      |
|  - Single source of truth (useSyncExternalStore)             |
+-------------------------------------------------------------+
                            |
                            v (Wails v2 IPC / Runtime Bridge)
+-------------------------------------------------------------+
|  Application & Binding Layer (main.go, app.go, app_multi.go)|
|  - Wails App bindings: validate inputs -> domain -> return  |
|  - Reactive runtime events (runtime.EventsEmit)             |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
|  Domain & Engine Layer (pkg/bot/, internal/multi/)          |
|  - Bot state machines, concurrency isolation (goroutines)   |
|  - Automation engines: combat, farming, navigation, party   |
|  - Multi-instance coordinator & scheduler                   |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
|  Infrastructure & Protocol Layer (pkg/network/, pkg/protocol)
|  - TCP packet framing, stream encryption/decryption         |
|  - Game protocol opcodes, serializers & packet handlers     |
|  - Proxy management & SoftEther VPN integration             |
|  - File-system atomic JSON persistence                      |
+-------------------------------------------------------------+
```

### Dependency Invariants
1. **Domain Isolation**: `pkg/bot/` and `pkg/protocol/` contain zero UI code, zero Wails webview imports, and zero frontend assumptions.
2. **Binding Discipline**: Methods exposed in `app.go` and `app_multi.go` are limited to input validation, delegating to domain engines, and returning typed payloads or errors.
3. **No Direct Webview Invocation in Presentation**: Components never access `window.go` directly; all backend calls are mediated by `src/lib/api/client.ts` and ingested into `src/lib/stores/`.
4. **Transport Decoupling**: Networking in `pkg/network/` abstracts raw TCP sockets, SOCKS5 proxies, and packet encryption. Switching network paths requires zero changes to bot automation state machines.

---

## 2. Bidirectional Data Flow

### Command Path (UI → Backend Intent)
```
User Interaction (Click / Input)
  ↓
Component dispatches intent to Store
  ↓
Store invokes typed wrapper in `src/lib/api/client.ts`
  ↓
Wails v2 RPC call `window.go.main.App.<Method>(args)`
  ↓
`app.go` / `app_multi.go` validates parameters
  ↓
Domain Service (`BotManager`, `MultiEngine`, `BotInstance`) executes logic
  ↓
Typed result returned (or error rejected) to API wrapper
  ↓
Store updates state snapshot
```

### Event Path (Backend → Frontend Reactive Sync)
```
Network Packet received by `pkg/network/` or periodic tick in `pkg/bot/`
  ↓
Bot instance updates internal state in isolated goroutine
  ↓
App / Engine emits typed event via `wailsRuntime.EventsEmit(ctx, "event://...", payload)`
  ↓
`src/lib/stores/eventBus.ts` receives payload over `window.runtime.EventsOn`
  ↓
Store updates reactive snapshot (`useSyncExternalStore`)
  ↓
React components re-render automatically (no polling)
```

---

## 3. Bot Concurrency & Isolation Model

- **Registry Concurrency**: `BotManager` maintains a thread-safe registry (`sync.RWMutex` protecting `map[string]*BotInstance`).
- **Instance Isolation**: Each `BotInstance` runs independent goroutines (packet reader, packet writer, automation tick loop) managed by Go `context.Context` cancellation.
- **No Cross-Bot State Contention**: Locking one bot instance mutex never blocks command execution or packet handling on another bot.
- **Worker Pools**: Bulk operations (start all, stop all, bulk settings) execute through bounded worker pools in `internal/multi/` preventing resource exhaustion.

---

## 4. Game Protocol & Modular Extensibility

- **Packet Handling**: `pkg/protocol/` defines packet parsers, packet builders, and opcode routing tables.
- **Automation Pipeline**: Sub-engines in `pkg/bot/` (`farm_engine.go`, `combat_engine.go`, `party_engine.go`, `pathfinder.go`) operate as composable modules attached to each bot instance.
- **Third-Party Integrations**: Features like SoftEther VPN, Discord webhooks, and local persistence reside in `internal/modules/` or dedicated infrastructure packages.
