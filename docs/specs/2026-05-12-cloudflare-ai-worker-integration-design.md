# Cloudflare AI Worker Integration Design

**Issue:** FP-sscpykry  
**Date:** 2026-05-12  
**Status:** Draft

---

## Problem

waveplan-mcp tools such as `waveplan_swim_refine` and `waveplan_swim_refine_run`
currently produce deterministic output with no AI-generated insight. Adding
inline AI augmentation — refinement suggestions, task summaries, anomaly
detection — requires a callable inference endpoint that:

- Is always available to the MCP server process (no local model)
- Requires no SDK dependency in the Go binary
- Can be independently deployed and versioned
- Is callable over plain HTTP

---

## Solution Overview

Deploy a **Cloudflare Worker** that wraps Workers AI inference and exposes a
single HTTP endpoint. The waveplan-mcp binary calls it via a thin Go HTTP
client. The worker is deployed and managed entirely through the Cloudflare
Workers REST API — no `wrangler` CLI.

```
MCP Client (Claude / agent)
        │
        │  MCP (stdio/SSE)
        ▼
 waveplan-mcp (Go)
        │
        │  POST /ask  (HTTP/JSON)
        ▼
 Cloudflare Worker (TypeScript)
        │
        │  Workers AI binding
        ▼
 @cf/meta/llama-3.1-8b-instruct
```

---

## Components

### 1. Cloudflare Worker

**Language:** TypeScript, compiled to a single ES module by esbuild.  
**Deployment:** REST API only — `PUT /accounts/:account_id/workers/scripts/:name`  
**AI model:** `@cf/meta/llama-3.1-8b-instruct-fp8` (default; configurable via
`AI_MODEL` secret). The deploy script validates the configured model against
the Workers AI model catalog at `GET /accounts/:id/ai/models/search` before
upload and fails fast on an unknown slug.
See: [Cloudflare model card](https://developers.cloudflare.com/workers-ai/models/llama-3.1-8b-instruct-fp8/).

#### Routes

| Method | Path      | Description                        |
|--------|-----------|------------------------------------|
| POST   | `/ask`    | Run AI inference on a prompt       |
| GET    | `/health` | Liveness check, returns `{"ok":true}` |

#### Request / Response

```json
// POST /ask
// Request
{ "prompt": "Summarise the following SWIM step: ..." }

// Response 200
{ "response": "This step configures the scheduler …", "model": "@cf/meta/llama-3.1-8b-instruct" }

// Response 400
{ "error": "missing prompt" }
```

#### Worker Env Interface

```typescript
interface Env {
  AI: Ai;               // Workers AI binding (declared in metadata)
  AI_MODEL: string;     // secret_text — model slug override
  AUTH_TOKEN: string;   // secret_text — optional bearer auth
}
```

#### Deployment Metadata (multipart `metadata` part)

```json
{
  "main_module": "worker.js",
  "compatibility_date": "2025-09-01",
  "bindings": [
    { "name": "AI", "type": "ai" }
  ]
}
```

Per the [Cloudflare multipart upload metadata docs](https://developers.cloudflare.com/workers/configuration/multipart-upload-metadata/),
`main_module` names the multipart form **part** that contains the entry-point
module — not an arbitrary filename. Therefore the script part must be uploaded
with `name="worker.js"` (matching `main_module`), `filename="worker.js"`, and
`Content-Type: application/javascript+module`. The metadata JSON is a separate
form part named `metadata` with `Content-Type: application/json`.

---

### 2. Go Client Package — `internal/aiclient`

A minimal HTTP client that the `WaveplanServer` holds as an optional dependency.

#### Package layout

```
internal/aiclient/
  client.go        — Client struct, Ask(), health check
  client_test.go   — httptest-based unit tests
```

#### API

```go
package aiclient

type Client struct {
    WorkerURL  string
    HTTPClient *http.Client
}

// NewClient returns nil if CF_WORKER_URL is unset.
func NewClient() *Client

// Ask sends prompt to the worker and returns the AI response text.
// Returns ("", nil) if the client is nil (feature disabled).
func (c *Client) Ask(ctx context.Context, prompt string) (string, error)
```

`NewClient` reads `CF_WORKER_URL` from the environment. If the variable is
absent the function returns `nil`; callers treat a nil client as "AI disabled"
and skip augmentation without error.

#### Configuration

| Env Var | Required | Description |
| --- | --- | --- |
| `CF_WORKER_URL` | No | Full worker URL, e.g. `https://ai.example.workers.dev`. Unset disables AI augmentation entirely. |
| `CF_AUTH_TOKEN` | Yes when `CF_WORKER_URL` is set | Bearer token matched against `AUTH_TOKEN` secret in worker. `NewClient` returns nil if `CF_WORKER_URL` is set but `CF_AUTH_TOKEN` is missing, with a warning log. |

---

### 3. Integration Points in waveplan-mcp

`WaveplanServer` gains an optional `ai *aiclient.Client` field, set at startup.

```go
type WaveplanServer struct {
    // existing fields ...
    ai *aiclient.Client  // nil when CF_WORKER_URL unset
}
```

#### Target tools (phase 1)

AI augmentation is **opt-in per call** via an `ai=true` tool argument. Default
behaviour of every tool remains fully deterministic. The `CF_WORKER_URL` env
var must also be set; both gates required.

| Tool | Integration |
| --- | --- |
| `waveplan_swim_refine_run` | Only when `ai=true`. After step execution, call `ai.Ask` with **step title + task ID only** (no stdout/stderr). AI summary returned as a sidecar field in the MCP tool response — **not** written to the SWIM journal. |
| `waveplan_get` | Only when `ai=true`. Build prompt outside the server mutex, run after the locked section returns. Result cached by `(task_id, plan_git_sha)` so repeated calls do not refetch. |

#### Determinism boundary

SWIM journals are append-only execution/recovery artifacts. AI text is
non-deterministic and must not enter them. Persistent AI output, if needed,
goes to a sidecar:

```
<journal>.json        — deterministic SWIM journal (unchanged schema)
<journal>.ai.json     — optional AI annotations, keyed by step ID
```

`internal/swim.JournalEvent` is **not** extended with AI fields.

#### Mutex discipline

`handleGet` and `handleSwimRefineRun` currently hold `s.mu` while assembling
results. The AI path must run **outside** the lock:

```go
// 1. Inside lock: snapshot state needed for the prompt.
s.mu.Lock()
snap := s.snapshotForAI(taskID)
s.mu.Unlock()

// 2. Outside lock: network call, may take seconds.
if s.ai != nil && req.AI {
    if cached, ok := s.aiCache.Get(snap.Key()); ok {
        result.AISummary = cached
    } else if summary, err := s.ai.Ask(ctx, buildPrompt(snap)); err == nil {
        s.aiCache.Put(snap.Key(), summary)
        result.AISummary = summary
    }
    // err logged but not propagated — AI is advisory
}
```

AI errors are logged and silently dropped; they never fail a tool call.

---

## Deployment Flow

```
scripts/deploy.sh
│
├── 1. esbuild src/worker.ts → dist/worker.js
│
├── 2. Validate AI_MODEL against GET /accounts/$ID/ai/models/search
│
├── 3. PUT /accounts/$CF_ACCOUNT_ID/workers/scripts/$WORKER_NAME
│      multipart:
│        - part name="worker.js" (matches metadata.main_module),
│          filename="worker.js", Content-Type: application/javascript+module
│        - part name="metadata", Content-Type: application/json
│
├── 4. PUT .../secrets  (AI_MODEL, AUTH_TOKEN)
│
├── 5. Enable workers.dev subdomain (if not already on a custom route):
│      POST /accounts/$ID/workers/scripts/$WORKER_NAME/subdomain
│      body: {"enabled": true}
│
└── 6. GET $CF_WORKER_URL/health  → verify live
```

The script is idempotent: re-running it updates the existing worker in-place.

### Public endpoint setup

Script upload alone does not expose a worker on the public internet. The
deploy script enables one of:

- **`workers.dev` subdomain** — default. Uses Cloudflare's
  [Worker Subdomain API](https://developers.cloudflare.com/api/resources/workers/subresources/scripts/subresources/subdomain/)
  to flip the script's subdomain on. Resulting URL:
  `https://$WORKER_NAME.$ACCOUNT_SUBDOMAIN.workers.dev`.
- **Custom route or domain** — opt-in via `CF_ROUTE_PATTERN`. The script
  attaches a route per the
  [Workers Routing docs](https://developers.cloudflare.com/workers/configuration/routing/).

`CF_WORKER_URL` is computed from whichever path was taken and written back to
`.env` for the Go client and test scripts.

---

## Security

- **AUTH_TOKEN is mandatory.** The worker rejects any request whose
  `Authorization: Bearer <token>` header does not match the `AUTH_TOKEN`
  secret. The deploy script refuses to deploy without `AUTH_TOKEN` set. The Go
  client refuses to call the worker without `CF_AUTH_TOKEN`. There is no
  unauthenticated path.
- **API_TOKEN scope** — Cloudflare token requires `Workers Scripts:Edit`,
  `Workers AI:Read`, and `Workers Routes:Edit` (only if using custom routes).
  Do not use the global API key.
- **Prompt content allowlist.** Prompts are constructed from a fixed allowlist
  of fields: SWIM step title, task ID, plan name, wave index. **Step stdout,
  stderr, environment variables, file paths, command argv, journal contents,
  plan file bodies, and tool error text are never sent.** A unit test in
  `internal/aiclient` asserts `buildPrompt` only references allowlisted fields
  on a fixture step containing secrets in stdout/stderr.
- **Size cap.** `Client.Ask` rejects prompts over 2 KiB before sending. The
  worker rejects any request body over 4 KiB.
- **No prompt logging.** Neither the worker nor the Go client logs prompt
  bodies. Only durations, status codes, and prompt byte counts are emitted.

---

## File Map

```
waveplan-mcp/                         # repo root (Go module)
  internal/
    aiclient/
      client.go
      client_test.go
  main.go                             # WaveplanServer gains ai field

cloudflare-worker/                    # sibling directory (new)
  src/
    worker.ts
    types.ts
  esbuild.config.js
  metadata.json
  scripts/
    deploy.sh
    test.sh
  .env.example
  package.json
  tsconfig.json
```

---

## Open Questions

1. **Model choice** — Default `@cf/meta/llama-3.1-8b-instruct-fp8`. Alternatives:
   `@cf/meta/llama-3.1-8b-instruct-fast`, `@cf/google/gemma-7b-it`,
   `@cf/mistral/mistral-7b-instruct-v0.1`. Deploy script validates against
   live model catalog.
2. **Streaming** — Workers AI supports streaming responses. Phase 1 uses
   non-streaming for simplicity; phase 2 could stream tokens back through the
   MCP SSE transport.
3. **Prompt engineering** — `handleSwimRefineRun` needs a prompt template.
   Template lives in `internal/aiclient/prompts.go` once stabilised, and must
   only reference allowlisted fields (see Security).
4. **Rate limiting** — Workers AI has free-tier limits. If waveplan is run in
   high-frequency batch mode, add a token bucket in `Client.Ask`.
5. **AI cache eviction** — In-process LRU keyed by `(task_id, plan_git_sha)`.
   Size cap and TTL TBD; start with 256 entries and no TTL since plan_git_sha
   already invalidates on plan change.

---

## Changelog

- **2026-05-12** Initial draft.
- **2026-05-12** Revision after review:
  - Fixed multipart upload contract: script part name must equal `main_module`.
  - Made AI augmentation explicitly opt-in via `ai=true` tool argument.
  - Removed AI output from SWIM journals; introduced `<journal>.ai.json` sidecar.
  - Moved AI calls outside `s.mu` to prevent server-wide lock contention.
  - Added `workers.dev` subdomain enablement and custom-route option to deploy flow.
  - Validated default model against current catalog; added pre-deploy model check.
  - Made `AUTH_TOKEN` and `CF_AUTH_TOKEN` mandatory; tightened prompt-content
    allowlist; added size caps and no-prompt-logging rule.
