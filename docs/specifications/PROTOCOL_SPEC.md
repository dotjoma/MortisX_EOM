# Echoes of Madrigal: Headless Network Protocol Specification

> **Version:** 2026.10  
> **Target:** Echoes of Madrigal (FlyFF Web/Godot 4.5.1 backend)  
> **Status:** Production Protocol Reference for `MadrigalMultibot`

---

## 1. Network Topology & Base Endpoints

The game client uses a hybrid networking model:
- **RESTful HTTP (JSON):** Account lifecycle, session management, character creation, slots, claims, and cloud saves.
- **WebSocket (JSON):** Real-time spatial entity synchronization, player movement, combat packets (`mhit`), chat, and world events.

### Base URLs:
* **HTTP Endpoint:** `https://echoesofmadrigal.com`
* **WebSocket Endpoint:** `wss://echoesofmadrigal.com/ws?ch={channel_id}`

### Common HTTP Headers:
```http
Content-Type: application/json
Accept-Encoding: identity
User-Agent: GodotEngine/4.5.1.stable.official (Windows)
```

### Authenticated HTTP Header:
```http
Authorization: Bearer {session_token}
```

---

## 2. Phase 1: Server & Channel Discovery

Before authenticating, the client fetches server health, online statistics, and channel capacity via `/api/hello`.

### Request:
```http
GET /api/hello HTTP/1.1
Host: echoesofmadrigal.com
```

### Response (`200 OK`):
```json
{
  "name": "Echoes of Madrigal",
  "version": "1.1.0",
  "online": 1278,
  "motd": "Welcome to Echoes of Madrigal! Be kind, have fun.",
  "channels": [
    { "id": 1, "name": "Channel 1 (PK)", "pk": true, "market": false, "online": 203, "max": 350 },
    { "id": 2, "name": "Channel 2 (Safe - No Giants)", "pk": false, "market": false, "online": 214, "max": 350 },
    { "id": 3, "name": "Channel 3 (Market)", "pk": false, "market": true, "online": 196, "max": 350 },
    { "id": 4, "name": "Channel 4 (Safe - No Giants)", "pk": false, "market": false, "online": 204, "max": 350 },
    { "id": 5, "name": "Channel 5 (Safe - No Giants)", "pk": false, "market": false, "online": 204, "max": 350 },
    { "id": 6, "name": "Channel 6 (Safe - No Giants)", "pk": false, "market": false, "online": 257, "max": 350 }
  ]
}
```

#### Channel Attributes:
* `id`: Numeric channel ID passed as query parameter to the WebSocket (`/ws?ch={id}`).
* `pk`: Boolean (`true`/`false`). Channel 1 is the PK channel where PvP is open outside towns.
* `market`: Boolean (`true`/`false`). Channel 3 is the dedicated Player Market channel.

---

## 3. Phase 2: Authentication & Registration

### A. Register New Account
```http
POST /api/register HTTP/1.1
Host: echoesofmadrigal.com
Content-Type: application/json

{
  "name": "bot_account01",
  "password": "Password123!"
}
```

#### Success Response (`200 OK`):
```json
{
  "account": {
    "key": "bot_account01",
    "name": "bot_account01",
    "slots": ["", "", ""],
    "auth": 0,
    "dp": 0,
    "vip": 0
  },
  "token": "7a8b9c0d1e2f34567890abcdef123456"
}
```

#### Error Responses (`400 Bad Request`):
* `{"error": "New accounts cannot be made from your network. If you think this is a mistake, contact us on our Facebook page or Discord."}` $\to$ Egress IP subnet blocked. Requires proxy / IP rotation.
* `{"error": "That account name is already taken."}` $\to$ Collision; retry with unique name.
* `{"error": "Pick an account name (at least 3 letters)."}` $\to$ Validation error.

---

### B. Login Existing Account
```http
POST /api/login HTTP/1.1
Host: echoesofmadrigal.com
Content-Type: application/json

{
  "name": "bot_account01",
  "password": "Password123!"
}
```

#### Success Response (`200 OK`):
Returns the same payload as register with current character names in `slots`.

#### Error Responses (`400 Bad Request`):
* `{"error": "Invalid account or password."}`
* `{"error": "This account is blocked."}` $\to$ Banned account.

---

### C. Session Token Resume
```http
POST /api/session HTTP/1.1
Host: echoesofmadrigal.com
Authorization: Bearer {session_token}
Content-Type: application/json

{}
```

Validates existing token without re-entering credentials. Returns account object.

---

## 4. Phase 3: Character Slots & Creation

Accounts have exactly **3 slots** (indices `0`, `1`, `2`) represented by `account.slots`.

### A. Create Character
Call `/api/char/create` on an empty slot index (`slots[i] == ""`):

#### Request:
```http
POST /api/char/create HTTP/1.1
Host: echoesofmadrigal.com
Authorization: Bearer {session_token}
Content-Type: application/json

{
  "slot": 0,
  "name": "TotoyBot"
}
```

#### Response (`200 OK`):
```json
{
  "ok": true,
  "account": {
    "key": "bot_account01",
    "name": "bot_account01",
    "slots": ["TotoyBot", "", ""],
    "auth": 0,
    "dp": 0,
    "vip": 0
  }
}
```

---

### B. Initial Character Data Schema (`chars/{acc_key}_{slot}.json`)
Immediately following character creation, the client generates the initial local state and submits the initial cloud save:

```json
{
  "name": "TotoyBot",
  "gender": "male",
  "face": 1,
  "hair": 1,
  "hair_color": "5a381f",
  "job": "vagrant",
  "level": 1,
  "exp": 0,
  "points": 0,
  "str": 15,
  "sta": 15,
  "dex": 15,
  "int": 15,
  "hp": 100,
  "mp": 100,
  "fp": 100,
  "penya": 200,
  "inv": [
    { "id": "II_GEN_FOO_INS_LOLLIPOP", "n": 10 },
    { "id": "II_GEN_FOO_ICE_ORANGEJUIICE", "n": 5 }
  ],
  "equip": {
    "weapon": "II_WEA_SWO_WOODEN",
    "suit": "II_ARM_M_VAG_SUIT01",
    "gloves": "II_ARM_M_VAG_GAUNTLET01",
    "boots": "II_ARM_M_VAG_BOOTS01"
  },
  "buffs": {
    "BEGINNER": {
      "params": [["DST_EXPERIENCE", 50], ["DST_CHANCEDROP", 30]],
      "until": 604800,
      "left": 604800
    }
  },
  "skills": {},
  "skill_points": 0,
  "pos": [3140.0, 100.0, -4250.0]
}
```

---

## 5. Phase 4: World Connection & WebSocket Lifecycle

### 1. WebSocket Endpoint Handshake
Connect WebSocket to:
```text
wss://echoesofmadrigal.com/ws?ch={channel_id}
```

### 2. Client Hello Packet (Sent immediately on socket open):
```json
{
  "t": "hello",
  "token": "{session_token}",
  "char": "TotoyBot",
  "lv": 1,
  "job": "vagrant",
  "map": "madrigal",
  "p": [3140.0, 100.0, -4250.0],
  "look": {
    "g": "male",
    "f": 1,
    "h": 1,
    "hc": "5a381f",
    "job": "vagrant",
    "eq": {
      "weapon": "II_WEA_SWO_WOODEN",
      "suit": "II_ARM_M_VAG_SUIT01",
      "gloves": "II_ARM_M_VAG_GAUNTLET01",
      "boots": "II_ARM_M_VAG_BOOTS01"
    }
  }
}
```

### 3. Server Welcome Packet (Server $\to$ Client):
```json
{
  "t": "welcome",
  "id": 4012345,
  "online": 1420,
  "pk": 0,
  "dsolo": 0
}
```
* `id`: Runtime entity ID assigned by the spatial grid.
* Client is now in active world state and receiving nearby mover and drop updates.

---

## 6. Critical Security & Anti-Detection Invariants

| Rule | Invariant Description | Penalty on Breach |
| :--- | :--- | :--- |
| **Cloud Save Interval** | Client must flush `/api/save` every **8 to 15 seconds** during gameplay. | WS Close `4006`: *"Your progress has not reached the server for several minutes"*. |
| **Server Drop Ledger** | Total Account Penya (`char.penya + bank.penya`) cannot exceed initial base + sum of loot packets. Mode 0 (Auto-Trash) must be used. | HTTP `400`: *"Save refused: items or Penya the server did not give out"* + client rollback. |
| **Leaderboard Shield** | **NEVER** call `POST /api/claim` with `k: "rec_put"`. Never report kill counts (`Index 29`) or damage (`Index 15`) to server record book. | Account enters Top 30 leaderboard $\to$ Manual GM audit & ban. |
| **Combat Emulation** | Send damage packets (`mhit`) matching reasonable level bounds. Never use 200k+ damage packets. | GM / Anti-cheat kill notice $\to$ Close code `4003` + IP ban. |
| **Client-side Invisibility** | The server checks `Auth >= 50` for `t: "gm", k: "invis"`. Non-GM accounts are never invisible to server or nearby clients. | False sense of security $\to$ live detection. |
