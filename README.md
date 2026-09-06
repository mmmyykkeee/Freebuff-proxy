# freebuff-go

Single-binary proxy exposing Freebuff (codebuff.com) free-tier models through
OpenAI-compatible and Anthropic-compatible APIs. Built from a study of
[Freebuff2API](https://github.com/Quorinex/Freebuff2API),
[freebuff-proxy](https://github.com/ferdiunal/freebuff-proxy) and
[free-buff-lol](https://github.com/notBlubbll/free-buff-lol) — with a live
model registry parsed from upstream's own TypeScript constants, so withdrawn
models (M3, DeepSeek V4 Pro, GLM 5.2, ox-alpha) are never served.

## Features

- **OpenAI-compatible**: `POST /v1/chat/completions` (stream + non-stream), `GET /v1/models`
- **Anthropic-compatible**: `POST /v1/messages` (stream + non-stream)
- **Multi-token pool**: round-robin across `AUTH_TOKENS`, per-token run/session state
- **Waiting-room aware**: polls the freebuff session queue, reports position, retries
- **Protocol-hardened**: mimics official CLI fingerprints (UA strings, run chains,
  context-pruner child runs, gemini dual parent/child runs, `cb_easp` stop token,
  Buffy admission marker)
- **Auto-recovery**: 429 → 3 retries (3s/6s/9s); invalid run → rotate + retry;
  invalid session → refresh + retry; 401 → 30-min token cooldown
- **Live model registry**: refreshes every 6h from upstream sources; hardcoded
  fallback if GitHub is unreachable

## Run

```sh
go build -o freebuff-go .
AUTH_TOKENS=<your-codebuff-authToken> ./freebuff-go
```

Get your `authToken` from `~/.config/manicode/credentials.json` after logging
into the codebuff CLI (or from the freebuff web session).

Point any client at `http://localhost:8080/v1`:

```sh
curl -s localhost:8080/v1/models | jq
curl -s localhost:8080/v1/chat/completions -d '{
  "model": "deepseek-v4-flash",
  "messages": [{"role": "user", "content": "hi"}]
}'
```

## Config

`config.json` (or env vars, which win):

```json
{
  "LISTEN_ADDR": ":8080",
  "UPSTREAM_BASE_URL": "https://www.codebuff.com",
  "AUTH_TOKENS": ["token-a", "token-b"],
  "API_KEYS": ["sk-proxy-1"],
  "ROTATION_INTERVAL": "6h",
  "REQUEST_TIMEOUT": "15m",
  "HTTP_PROXY": ""
}
```

| Key | Meaning |
|---|---|
| `AUTH_TOKENS` | Upstream codebuff auth tokens (pool) |
| `API_KEYS` | Optional keys clients must present (`Authorization: Bearer`); empty = open |
| `ROTATION_INTERVAL` | Restart agent runs this often (CLI rotates per session) |
| `HTTP_PROXY` | Outbound proxy for upstream calls |

## Endpoints

- `GET /healthz` — uptime + per-token pool/session snapshots
- `GET /v1/models` — live model list (canonical ids + short aliases)
- `POST /v1/chat/completions` — OpenAI shape
- `POST /v1/messages` — Anthropic shape
- `POST /v1/messages/count_tokens` — Anthropic token estimate

## Caveats

- Free tier is rate-limited and queued; expect waiting-room positions under load.
- Sessions lock to one model per token; switching models ends + recreates the session.
- Upstream may withdraw models or change the protocol at any time; the registry
  tracks upstream constants, but a protocol break needs a code fix.
- Using the free tier through a proxy may violate codebuff's ToS. Your token, your risk.
