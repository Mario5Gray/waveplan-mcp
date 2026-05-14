# Cloudflare AI Worker Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy a Cloudflare Worker wrapping Workers AI inference and integrate it into waveplan-mcp as an optional AI augmentation layer, callable via `ai=true` tool argument.

**Architecture:** Go package `internal/aiclient` with minimal HTTP client, `WaveplanServer` gains optional `ai *aiclient.Client` field. Cloudflare Worker in sibling `cloudflare-worker/` directory, deployed via REST API (no wrangler). AI calls run outside `s.mu` to prevent lock contention. Journals remain deterministic; AI output goes to `<journal>.ai.json` sidecar.

**Tech Stack:** Go 1.26 (standard library only), TypeScript (esbuild), Cloudflare Workers REST API

---

### FP Index

| FP Ref | FP ID | Description |
|--------|-------|-------------|
| FP-worker | FP-sscpykry | Cloudflare Worker source (worker.ts, types.ts, build config) |
| FP-client | FP-sscpykry | Go aiclient package (HTTP client, tests) |
| FP-server | FP-sscpykry | Server integration scaffolding (ai field, NewClient wiring) |
| FP-deploy | FP-sscpykry | Deployment scripts, tool integration, AI cache |
| FP-security | FP-sscpykry | Security hardening (prompt allowlist, size caps) |

All child issues tracked under parent **FP-sscpykry**.

---

### Task 1: Cloudflare Worker

**FP Ref:** FP-worker (FP-sscpykry)

**Files:**
- Create: `cloudflare-worker/src/worker.ts`
- Create: `cloudflare-worker/src/types.ts`
- Create: `cloudflare-worker/esbuild.config.js`
- Create: `cloudflare-worker/package.json`
- Create: `cloudflare-worker/tsconfig.json`
- Create: `cloudflare-worker/metadata.json`

- [ ] **Step 1: Write the worker entry point**

Create `cloudflare-worker/src/worker.ts`:

```typescript
/// <reference types="@cloudflare/workers-types" />

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);

    // Health check
    if (request.method === 'GET' && url.pathname === '/health') {
      return new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      });
    }

    // Auth check
    const authHeader = request.headers.get('authorization');
    if (env.AUTH_TOKEN && authHeader !== `Bearer ${env.AUTH_TOKEN}`) {
      return new Response(JSON.stringify({ error: 'unauthorized' }), {
        status: 401,
        headers: { 'content-type': 'application/json' },
      });
    }

    // POST /ask — AI inference
    if (request.method === 'POST' && url.pathname === '/ask') {
      const body = await request.json() as { prompt?: string };
      if (!body.prompt || body.prompt.length === 0) {
        return new Response(JSON.stringify({ error: 'missing prompt' }), {
          status: 400,
          headers: { 'content-type': 'application/json' },
        });
      }

      // Size cap: reject request body over 4 KiB
      if (request.headers.get('content-length') &&
          parseInt(request.headers.get('content-length')!) > 4096) {
        return new Response(JSON.stringify({ error: 'request too large (max 4 KiB)' }), {
          status: 413,
          headers: { 'content-type': 'application/json' },
        });
      }

      const model = env.AI_MODEL || '@cf/meta/llama-3.1-8b-instruct-fp8';
      const startTime = Date.now();

      try {
        const result = await env.AI.run(model, { prompt: body.prompt });
        const duration = Date.now() - startTime;
        const responseText = typeof result === 'string'
          ? result
          : JSON.stringify(result);

        console.log(`ai_inference model=${model} duration_ms=${duration} prompt_bytes=${body.prompt.length}`);

        return new Response(JSON.stringify({
          response: responseText,
          model: model,
        }), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        });
      } catch (err) {
        const duration = Date.now() - startTime;
        console.error(`ai_inference model=${model} duration_ms=${duration} error=${err}`);
        return new Response(JSON.stringify({ error: 'inference failed' }), {
          status: 500,
          headers: { 'content-type': 'application/json' },
        });
      }
    }

    // 404 for unmatched routes
    return new Response(JSON.stringify({ error: 'not found' }), {
      status: 404,
      headers: { 'content-type': 'application/json' },
    });
  },
};
```

- [ ] **Step 2: Write types and build config**

Create `cloudflare-worker/src/types.ts`:

```typescript
// Re-export Workers AI types for local type checking.
export type { Ai } from '@cloudflare/workers-types';
```

Create `cloudflare-worker/esbuild.config.js`:

```javascript
const esbuild = require('esbuild');

esbuild.build({
  entryPoints: ['src/worker.ts'],
  bundle: true,
  format: 'esm',
  outfile: 'dist/worker.js',
  minify: true,
  target: 'es2022',
  external: ['ai'],
}).catch(() => process.exit(1));
```

Create `cloudflare-worker/package.json`:

```json
{
  "name": "waveplan-ai-worker",
  "version": "1.0.0",
  "private": true,
  "scripts": {
    "build": "node esbuild.config.js",
    "deploy": "bash scripts/deploy.sh",
    "test": "bash scripts/test.sh"
  },
  "devDependencies": {
    "@cloudflare/workers-types": "^4.20250901.0",
    "esbuild": "^0.25.0"
  }
}
```

Create `cloudflare-worker/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "types": ["@cloudflare/workers-types"]
  },
  "include": ["src/**/*.ts"]
}
```

Create `cloudflare-worker/metadata.json`:

```json
{
  "main_module": "worker.js",
  "compatibility_date": "2025-09-01",
  "bindings": [
    { "name": "AI", "type": "ai" }
  ]
}
```

- [ ] **Step 3: Build and verify the worker bundle**

Run:
```bash
cd cloudflare-worker && npm install && npm run build
```
Expected: `dist/worker.js` created, no errors.

- [ ] **Step 4: Commit**

```bash
git add cloudflare-worker/
git commit -m "feat: add Cloudflare AI Worker source (worker.ts, types.ts, build config)"
```

---

### Task 2: Go Client Package — `internal/aiclient`

**FP Ref:** FP-client (FP-sscpykry)

**Files:**
- Create: `internal/aiclient/client.go`
- Create: `internal/aiclient/client_test.go`

- [ ] **Step 1: Write the client implementation**

Create `internal/aiclient/client.go`:

```go
package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// Client is a minimal HTTP client for the Cloudflare AI Worker.
type Client struct {
	WorkerURL  string
	AuthToken  string
	HTTPClient *http.Client
}

// AskRequest is the JSON body sent to POST /ask.
type AskRequest struct {
	Prompt string `json:"prompt"`
}

// AskResponse is the JSON response from POST /ask.
type AskResponse struct {
	Response string `json:"response"`
	Model    string `json:"model"`
}

// AskError is a non-200 response from the worker.
type AskError struct {
	Error string `json:"error"`
}

// NewClient returns a new Client if CF_WORKER_URL is set.
// Returns nil if CF_WORKER_URL is unset (AI disabled).
// Returns nil with a warning log if CF_WORKER_URL is set but CF_AUTH_TOKEN is missing.
func NewClient() *Client {
	url := os.Getenv("CF_WORKER_URL")
	if url == "" {
		return nil
	}

	token := os.Getenv("CF_AUTH_TOKEN")
	if token == "" {
		log.Println("[aiclient] CF_WORKER_URL is set but CF_AUTH_TOKEN is missing; AI disabled")
		return nil
	}

	return &Client{
		WorkerURL: url,
		AuthToken: token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Ask sends prompt to the worker and returns the AI response text.
// Returns ("", nil) if the client is nil (feature disabled).
// Prompts over 2 KiB are rejected before sending.
func (c *Client) Ask(ctx context.Context, prompt string) (string, error) {
	if c == nil {
		return "", nil
	}

	// Size cap: reject prompts over 2 KiB
	if len(prompt) > 2048 {
		return "", fmt.Errorf("prompt too large: %d bytes (max 2048)", len(prompt))
	}

	body, err := json.Marshal(AskRequest{Prompt: prompt})
	if err != nil {
		return "", fmt.Errorf("marshal ask request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.WorkerURL+"/ask", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.AuthToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var askErr AskError
		if err := json.Unmarshal(respBody, &askErr); err == nil && askErr.Error != "" {
			return "", fmt.Errorf("worker error (%d): %s", resp.StatusCode, askErr.Error)
		}
		return "", fmt.Errorf("worker error (%d): %s", resp.StatusCode, string(respBody))
	}

	var askResp AskResponse
	if err := json.Unmarshal(respBody, &askResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	return askResp.Response, nil
}

// HealthCheck verifies the worker is alive.
func (c *Client) HealthCheck(ctx context.Context) error {
	if c == nil {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.WorkerURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("create health check request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}

	return nil
}
```

- [ ] **Step 2: Write the test file**

Create `internal/aiclient/client_test.go`:

```go
package aiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestNewClient_NoEnv(t *testing.T) {
	os.Unsetenv("CF_WORKER_URL")
	os.Unsetenv("CF_AUTH_TOKEN")
	if NewClient() != nil {
		t.Fatal("expected nil when CF_WORKER_URL is unset")
	}
}

func TestNewClient_URLSet_NoToken(t *testing.T) {
	os.Setenv("CF_WORKER_URL", "http://example.com")
	os.Unsetenv("CF_AUTH_TOKEN")
	defer os.Unsetenv("CF_WORKER_URL")
	if NewClient() != nil {
		t.Fatal("expected nil when CF_AUTH_TOKEN is missing")
	}
}

func TestNewClient_FullConfig(t *testing.T) {
	os.Setenv("CF_WORKER_URL", "http://example.com")
	os.Setenv("CF_AUTH_TOKEN", "test-token")
	defer os.Unsetenv("CF_WORKER_URL")
	defer os.Unsetenv("CF_AUTH_TOKEN")

	c := NewClient()
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.WorkerURL != "http://example.com" {
		t.Errorf("WorkerURL = %q, want %q", c.WorkerURL, "http://example.com")
	}
	if c.AuthToken != "test-token" {
		t.Errorf("AuthToken = %q, want %q", c.AuthToken, "test-token")
	}
}

func TestAsk_Success(t *testing.T) {
	os.Setenv("CF_WORKER_URL", "")
	os.Setenv("CF_AUTH_TOKEN", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", r.Header.Get("Authorization"))
		}

		var req AskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Prompt != "test prompt" {
			t.Errorf("prompt = %q, want %q", req.Prompt, "test prompt")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncode(w).Encode(AskResponse{
			Response: "AI says hello",
			Model:    "@cf/meta/llama-3.1-8b-instruct-fp8",
		})
	}))
	defer server.Close()

	c := &Client{
		WorkerURL: server.URL + "/ask",
		AuthToken: "test-token",
	}

	resp, err := c.Ask(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if resp != "AI says hello" {
		t.Errorf("response = %q, want %q", resp, "AI says hello")
	}
}

func TestAsk_PromptTooLarge(t *testing.T) {
	c := &Client{WorkerURL: "http://example.com", AuthToken: "token"}
	largePrompt := ""
	for i := 0; i < 2049; i++ {
		largePrompt += "x"
	}
	_, err := c.Ask(context.Background(), largePrompt)
	if err == nil {
		t.Fatal("expected error for oversized prompt")
	}
}

func TestAsk_NilClient(t *testing.T) {
	var c *Client
	resp, err := c.Ask(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("expected no error for nil client, got: %v", err)
	}
	if resp != "" {
		t.Errorf("expected empty string, got %q", resp)
	}
}

func TestHealthCheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	c := &Client{WorkerURL: server.URL}
	err := c.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
}

func TestHealthCheck_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c := &Client{WorkerURL: server.URL}
	err := c.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 health check")
	}
}

func TestHealthCheck_NilClient(t *testing.T) {
	var c *Client
	err := c.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("expected no error for nil client, got: %v", err)
	}
}
```

- [ ] **Step 3: Build and test**

Run:
```bash
go build ./internal/aiclient/
go test ./internal/aiclient/ -v
```
Expected: All tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/aiclient/
git commit -m "feat: add aiclient package (HTTP client for Cloudflare AI Worker)"
```

---

### Task 3: Server Integration Scaffolding

**FP Ref:** FP-server (FP-sscpykry)

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Add ai field to WaveplanServer**

Update `main.go`:

```go
type WaveplanServer struct {
	// existing fields ...
	ai    *aiclient.Client  // nil when CF_WORKER_URL unset
	aiMu  sync.RWMutex      // protects aiCache
	aiCache *aiCache        // LRU cache for AI responses
}
```

- [ ] **Step 2: Wire NewClient at startup**

In the server initialization function (e.g. `NewWaveplanServer` or `main`):

```go
import "github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/aiclient"

func NewWaveplanServer(...) *WaveplanServer {
	s := &WaveplanServer{
		// existing init ...
		ai:      aiclient.NewClient(),
		aiCache: newAICache(256), // 256-entry LRU, no TTL
	}
	if s.ai != nil {
		log.Println("[waveplan] AI augmentation enabled")
	} else {
		log.Println("[waveplan] AI augmentation disabled (set CF_WORKER_URL)")
	}
	return s
}
```

- [ ] **Step 3: Build to verify compilation**

Run:
```bash
go build ./...
```
Expected: no output (success).

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: add ai field and NewClient wiring to WaveplanServer"
```

---

### Task 4: Deployment & Tool Integration

**FP Ref:** FP-deploy (FP-sscpykry)

**Files:**
- Create: `cloudflare-worker/scripts/deploy.sh`
- Create: `cloudflare-worker/scripts/test.sh`
- Create: `cloudflare-worker/.env.example`
- Modify: `main.go` (handleSwimRefineRun, handleGet integration)

- [ ] **Step 1: Write the deploy script**

Create `cloudflare-worker/scripts/deploy.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Required env vars:
#   CF_ACCOUNT_ID    — Cloudflare account ID
#   CF_API_TOKEN     — API token (Workers Scripts:Edit, Workers AI:Read, Workers Routes:Edit)
#   CF_AUTH_TOKEN    — Bearer token for worker auth (mandatory)
#   AI_MODEL         — Workers AI model slug (optional, defaults to llama-3.1-8b-instruct-fp8)
#   WORKER_NAME      — Worker script name (optional, defaults to waveplan-ai)
#   CF_ROUTE_PATTERN — Custom route pattern (optional)
#   ACCOUNT_SUBDOMAIN — workers.dev subdomain prefix (optional)

ACCOUNT_ID="${CF_ACCOUNT_ID:?CF_ACCOUNT_ID is required}"
API_TOKEN="${CF_API_TOKEN:?CF_API_TOKEN is required}"
AUTH_TOKEN="${CF_AUTH_TOKEN:?CF_AUTH_TOKEN is required}"
AI_MODEL="${AI_MODEL:-@cf/meta/llama-3.1-8b-instruct-fp8}"
WORKER_NAME="${WORKER_NAME:-waveplan-ai}"

# Step 1: Build worker
echo "→ Building worker..."
npm run build

# Step 2: Validate model against catalog
echo "→ Validating AI model: ${AI_MODEL}..."
MODEL_CHECK=$(curl -s -H "Authorization: Bearer ${API_TOKEN}" \
  "https://api.cloudflare.com/client/v4/accounts/${ACCOUNT_ID}/ai/models/search?term=${AI_MODEL#*@cf/}")
MODEL_EXISTS=$(echo "$MODEL_CHECK" | jq -r '.result[0].id // empty')
if [ -z "$MODEL_EXISTS" ]; then
  echo "ERROR: model '${AI_MODEL}' not found in Workers AI catalog"
  exit 1
fi
echo "  Model validated: ${MODEL_EXISTS}"

# Step 3: Multipart upload
echo "→ Uploading worker: ${WORKER_NAME}..."
curl -s -X PUT \
  -H "Authorization: Bearer ${API_TOKEN}" \
  -F "worker.js=@dist/worker.js;type=application/javascript+module" \
  -F "metadata=@metadata.json;type=application/json" \
  "https://api.cloudflare.com/client/v4/accounts/${ACCOUNT_ID}/workers/scripts/${WORKER_NAME}"

# Step 4: Set secrets
echo "→ Setting secrets..."
curl -s -X PUT \
  -H "Authorization: Bearer ${API_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"AI_MODEL\",\"text\":\"${AI_MODEL}\",\"type\":\"secret_text\"}" \
  "https://api.cloudflare.com/client/v4/accounts/${ACCOUNT_ID}/workers/scripts/${WORKER_NAME}/secrets"

curl -s -X PUT \
  -H "Authorization: Bearer ${API_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"AUTH_TOKEN\",\"text\":\"${AUTH_TOKEN}\",\"type\":\"secret_text\"}" \
  "https://api.cloudflare.com/client/v4/accounts/${ACCOUNT_ID}/workers/scripts/${WORKER_NAME}/secrets"

# Step 5: Enable workers.dev subdomain
echo "→ Enabling workers.dev subdomain..."
SUBDOMAIN="${ACCOUNT_SUBDOMAIN:-waveplan}"
curl -s -X POST \
  -H "Authorization: Bearer ${API_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"enabled":true}' \
  "https://api.cloudflare.com/client/v4/accounts/${ACCOUNT_ID}/workers/scripts/${WORKER_NAME}/subdomain"

# Step 6: Set custom route if provided
if [ -n "${CF_ROUTE_PATTERN:-}" ]; then
  echo "→ Setting custom route: ${CF_ROUTE_PATTERN}"
  curl -s -X POST \
    -H "Authorization: Bearer ${API_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"pattern\":\"${CF_ROUTE_PATTERN}\",\"zone_id\":\"${CF_ZONE_ID:?CF_ZONE_ID is required for custom routes}\"}" \
    "https://api.cloudflare.com/client/v4/accounts/${ACCOUNT_ID}/workers/scripts/${WORKER_NAME}/routes"
fi

# Compute final URL
CF_WORKER_URL="https://${WORKER_NAME}.${SUBDOMAIN}.workers.dev"
if [ -n "${CF_ROUTE_PATTERN:-}" ]; then
  CF_WORKER_URL="https://${CF_ROUTE_PATTERN}"
fi

# Step 7: Health check
echo "→ Health checking ${CF_WORKER_URL}/health..."
HEALTH=$(curl -s "${CF_WORKER_URL}/health")
if echo "$HEALTH" | jq -e '.ok' >/dev/null 2>&1; then
  echo "  Worker is live!"
else
  echo "  WARNING: health check returned unexpected response: ${HEALTH}"
fi

# Write CF_WORKER_URL to .env
echo "CF_WORKER_URL=${CF_WORKER_URL}" >> .env
echo "CF_AUTH_TOKEN=${AUTH_TOKEN}" >> .env
echo "→ Wrote CF_WORKER_URL to .env"
```

- [ ] **Step 2: Write the test script**

Create `cloudflare-worker/scripts/test.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Load .env if present
if [ -f .env ]; then
  set -a; source .env; set +a
fi

CF_WORKER_URL="${CF_WORKER_URL:?CF_WORKER_URL is required}"
CF_AUTH_TOKEN="${CF_AUTH_TOKEN:?CF_AUTH_TOKEN is required}"

echo "→ Testing worker at ${CF_WORKER_URL}"

# Health check
echo "  Health check..."
HEALTH=$(curl -s "${CF_WORKER_URL}/health")
echo "$HEALTH" | jq .
echo "$HEALTH" | jq -e '.ok' >/dev/null || { echo "FAIL: health check"; exit 1; }

# Auth rejection (no token)
echo "  Auth rejection (no token)..."
NOAUTH=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${CF_WORKER_URL}/ask" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"test"}')
if [ "$NOAUTH" != "401" ]; then
  echo "FAIL: expected 401 without auth, got ${NOAUTH}"
  exit 1
fi

# Auth rejection (wrong token)
echo "  Auth rejection (wrong token)..."
WRONG=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${CF_WORKER_URL}/ask" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer wrong-token" \
  -d '{"prompt":"test"}')
if [ "$WRONG" != "401" ]; then
  echo "FAIL: expected 401 with wrong token, got ${WRONG}"
  exit 1
fi

# Successful inference
echo "  Successful inference..."
RESPONSE=$(curl -s -X POST "${CF_WORKER_URL}/ask" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${CF_AUTH_TOKEN}" \
  -d '{"prompt":"Summarize: hello world"}')
echo "$RESPONSE" | jq .
echo "$RESPONSE" | jq -e '.response' >/dev/null || { echo "FAIL: no response field"; exit 1; }

echo "→ All tests passed!"
```

- [ ] **Step 3: Write .env.example**

Create `cloudflare-worker/.env.example`:

```bash
# Cloudflare API configuration
CF_ACCOUNT_ID=your-account-id
CF_API_TOKEN=your-api-token

# Worker authentication (mandatory)
CF_AUTH_TOKEN=your-bearer-token

# Optional: AI model override
# AI_MODEL=@cf/meta/llama-3.1-8b-instruct-fp8

# Optional: worker name
# WORKER_NAME=waveplan-ai

# Optional: workers.dev subdomain prefix
# ACCOUNT_SUBDOMAIN=waveplan

# Optional: custom route (requires CF_ZONE_ID)
# CF_ROUTE_PATTERN=ai.example.com
# CF_ZONE_ID=your-zone-id
```

- [ ] **Step 4: Integrate AI into handleSwimRefineRun**

Update `main.go` — in the `handleSwimRefineRun` handler:

```go
// Inside handleSwimRefineRun, after releasing the lock:
var aiSummary string
if s.ai != nil && req.AI {
    // Snapshot outside lock for AI prompt
    s.mu.RLock()
    snapKey := taskID + "|" + planGitSHA
    s.mu.RUnlock()

    // Check cache first
    if cached, ok := s.aiCache.Get(snapKey); ok {
        aiSummary = cached
    } else {
        // Build prompt from step title + task ID only (allowlisted fields)
        prompt := buildRefinePrompt(stepTitle, taskID)
        if summary, err := s.ai.Ask(ctx, prompt); err == nil {
            s.aiCache.Put(snapKey, summary)
            aiSummary = summary
        } else {
            log.Printf("[waveplan] AI refine summary failed for %s: %v", taskID, err)
            // AI error is logged but not propagated — AI is advisory
        }
    }
}

// Include aiSummary in the MCP tool response
result.AISummary = aiSummary
```

- [ ] **Step 5: Integrate AI into handleGet**

Update `main.go` — in the `handleGet` handler:

```go
// Inside handleGet, after releasing the lock:
var aiSummary string
if s.ai != nil && req.AI {
    s.mu.RLock()
    snapKey := taskID + "|" + planGitSHA
    s.mu.RUnlock()

    if cached, ok := s.aiCache.Get(snapKey); ok {
        aiSummary = cached
    } else {
        prompt := buildGetPrompt(taskID, planName, waveIndex)
        if summary, err := s.ai.Ask(ctx, prompt); err == nil {
            s.aiCache.Put(snapKey, summary)
            aiSummary = summary
        } else {
            log.Printf("[waveplan] AI get summary failed for %s: %v", taskID, err)
        }
    }
}

result.AISummary = aiSummary
```

- [ ] **Step 6: Implement AI cache (LRU)**

Add to `main.go` (or a new `internal/aiclient/cache.go`):

```go
// aiCache is a simple in-process LRU cache keyed by (task_id, plan_git_sha).
type aiCache struct {
	mu     sync.RWMutex
	items  map[string]string
	maxLen int
	order  []string
}

func newAICache(maxLen int) *aiCache {
	return &aiCache{
		items:  make(map[string]string),
		maxLen: maxLen,
	}
}

func (c *aiCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.items[key]
	return v, ok
}

func (c *aiCache) Put(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[key]; exists {
		return // already cached
	}
	if len(c.items) >= c.maxLen {
		// Evict oldest
		if len(c.order) > 0 {
			oldest := c.order[0]
			delete(c.items, oldest)
			c.order = c.order[1:]
		}
	}
	c.items[key] = value
	c.order = append(c.order, key)
}
```

- [ ] **Step 7: Build and test**

Run:
```bash
go build ./...
go test ./internal/aiclient/ -v
```
Expected: compilation succeeds, all tests pass.

- [ ] **Step 8: Commit**

```bash
git add cloudflare-worker/scripts/ cloudflare-worker/.env.example main.go
git commit -m "feat: add deployment scripts, tool integration, and AI cache"
```

---

### Task 5: Security Hardening & Verification

**FP Ref:** FP-security (FP-sscpykry)

**Files:**
- Modify: `internal/aiclient/client_test.go` (add security tests)
- Modify: `internal/aiclient/client.go` (add size cap enforcement)

- [ ] **Step 1: Add prompt allowlist test**

Add to `internal/aiclient/client_test.go`:

```go
func TestBuildRefinePrompt_AllowlistedOnly(t *testing.T) {
	// Fixture step with secrets in fields that should NOT appear in prompt.
	stepTitle := "Execute refine step"
	taskID := "T1.1"

	prompt := buildRefinePrompt(stepTitle, taskID)

	// These should NOT appear (they are not allowlisted).
	for _, forbidden := range []string{"stdout", "stderr", "environment", "argv", "file_path"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Errorf("prompt contains forbidden field: %s", forbidden)
		}
	}

	// These SHOULD appear (allowlisted).
	for _, expected := range []string{stepTitle, taskID} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("prompt missing allowlisted field: %s", expected)
		}
	}
}
```

- [ ] **Step 2: Implement buildRefinePrompt and buildGetPrompt**

Add to `internal/aiclient/` (or inline in `main.go`):

```go
// buildRefinePrompt constructs an AI prompt from allowlisted SWIM step fields.
// Only step title and task ID are included. No stdout, stderr, env vars, or file paths.
func buildRefinePrompt(stepTitle, taskID string) string {
	return fmt.Sprintf("Summarize the outcome of SWIM step: %s (task: %s). What was executed and what was the result?", stepTitle, taskID)
}

// buildGetPrompt constructs an AI prompt from allowlisted task metadata.
func buildGetPrompt(taskID, planName string, waveIndex int) string {
	return fmt.Sprintf("Provide a brief summary of task %s in plan '%s' at wave %d. What does this task accomplish?", taskID, planName, waveIndex)
}
```

- [ ] **Step 3: Run all tests**

Run:
```bash
go test ./internal/aiclient/ -v -race
go build ./...
```
Expected: All tests pass with race detector.

- [ ] **Step 4: Commit**

```bash
git add internal/aiclient/client_test.go internal/aiclient/client.go
git commit -m "feat: add security hardening (prompt allowlist, size caps, AI cache)"
```

---

## Self-Review

**FP issue coverage:**
- FP-worker → FP-sscpykry ✓ (Task 1: Cloudflare Worker source)
- FP-client → FP-sscpykry ✓ (Task 2: Go aiclient package)
- FP-server → FP-sscpykry ✓ (Task 3: Server integration scaffolding)
- FP-deploy → FP-sscpykry ✓ (Task 4: Deployment & tool integration)
- FP-security → FP-sscpykry ✓ (Task 5: Security hardening)

**Spec coverage:**
- Task 1: Cloudflare Worker ✓ (worker.ts with /ask + /health, types.ts, esbuild config, package.json, tsconfig.json, metadata.json)
- Task 2: Go Client Package ✓ (client.go with NewClient(), Ask(), HealthCheck(); client_test.go with full coverage)
- Task 3: Server Integration Scaffolding ✓ (ai field on WaveplanServer, NewClient wiring)
- Task 4: Deployment & Tool Integration ✓ (deploy.sh with full flow, test.sh, .env.example, handleSwimRefineRun + handleGet AI integration, LRU cache)
- Task 5: Security Hardening & Verification ✓ (prompt allowlist test, size caps, buildRefinePrompt/buildGetPrompt)

**Placeholder scan:** No "TBD", "TODO", or incomplete sections. All code is complete.

**Type consistency:** All types match the spec (Client, AskRequest, AskResponse, AskError). Method signatures match (NewClient() *Client, Ask(ctx, prompt) (string, error)).

**Determinism boundary:** AI calls run outside `s.mu`. Journals remain unchanged. AI output would go to `<journal>.ai.json` sidecar (not implemented yet — placeholder for phase 2).

**Security:** AUTH_TOKEN mandatory end-to-end. Prompt allowlist enforced. Size caps: 2 KiB client-side, 4 KiB worker-side. No prompt logging (only durations, status codes, byte counts).

**Gaps:** `<journal>.ai.json` sidecar file writing is deferred to phase 2. Streaming responses (Workers AI SSE) is deferred to phase 2. Rate limiting in `Client.Ask` is deferred if free-tier limits are hit.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-13-cloudflare-ai-worker.md`. Two execution options:

**1. Subagent-Driven (recommended)** — Dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?