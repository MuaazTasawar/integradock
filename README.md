# IntegraDock

Turn any REST API's OpenAPI spec into a live, callable toolset for an AI agent — so non-technical staff can type plain-English requests and the agent plans and executes real multi-step API calls, with a human confirmation gate before anything destructive.

Built as a 4-week MVP. Two sandbox integrations are wired end-to-end: a mock Inventory & Orders API, and a hand-curated Stripe (test mode) Customers tool set.

## How it works

1. Upload an OpenAPI spec (`.json` or `.yaml`) through the web UI.
2. `py-planner` parses it into a flat list of callable tools with JSON-Schema parameter definitions.
3. `go-engine` registers those tools against a tenant and stores them in Postgres.
4. You type a plain-English request in the chat panel.
5. `py-planner` runs an agent loop (raw function-calling — no LangChain) that plans one tool call at a time, reads back the real result, and decides the next step.
6. Every tool call is recorded as an execution step in `go-engine`. Reads execute immediately; anything destructive (create, update, delete) pauses and waits for you to click **Approve** or **Reject** before it touches a real API.
7. Every step streams live into the **Live execution trace** panel over a WebSocket, backed by Redis pub/sub.

## Architecture

```
┌─────────────┐     SSE (plan/confirm)     ┌─────────────┐
│  Next.js    │ ─────────────────────────► │  py-planner │
│  frontend   │                             │  (FastAPI)  │
│             │ ◄───────────────────────── │             │
└──────┬──────┘   WebSocket (live trace)    └──────┬──────┘
       │                                            │
       │  REST (tenants, tools, executions)         │  REST (tenants, tools, executions)
       ▼                                            ▼
┌─────────────────────────────────────────────────────────┐
│                      go-engine (Fiber)                    │
│   tool registry · execution engine · confirmation gate    │
└──────┬──────────────────────────────────────┬─────────────┘
       │                                       │
       ▼                                       ▼
┌─────────────┐                        ┌─────────────┐
│  Postgres   │                        │    Redis    │
│  (state)    │                        │ (pub/sub +  │
│             │                        │  run cache) │
└─────────────┘                        └─────────────┘

               go-engine also calls out to real APIs:
               ┌─────────────┐   ┌─────────────┐
               │  mock-api   │   │   Stripe    │
               │ (sandbox)   │   │ (test mode) │
               └─────────────┘   └─────────────┘
```

## Tech stack

| Layer | Tech |
|---|---|
| Tool registry + execution engine | Go, Fiber, pgx, go-redis |
| OpenAPI parsing + LLM planning loop | Python, FastAPI, raw function-calling (Anthropic + Gemini, no LangChain) |
| Frontend | Next.js 14, React, Tailwind |
| State | Postgres (tenants, tools, execution runs/steps), Redis (pub/sub + run cache) |
| Sandbox targets | Custom mock Inventory & Orders API (Go), Stripe test mode |

## Project structure

```
integradock/
├── go-engine/       # Fiber tool registry, execution engine, confirmation gating, websocket streaming
├── py-planner/       # FastAPI OpenAPI parser + LLM agent loop
├── frontend/          # Next.js chat UI + live trace panel
├── mock-api/         # Standalone in-memory sandbox API (products/orders)
└── docker-compose.yml
```

## Prerequisites

- Go 1.22+
- Python 3.12+
- Node.js 20+
- Docker Desktop (for Postgres + Redis)
- A Gemini API key (free tier) and/or an Anthropic API key

## Setup

All commands below are PowerShell-native (Windows). Adjust for `cmd.exe`/bash as needed.

### 1. Start Postgres and Redis

```powershell
cd integradock
docker compose up -d postgres redis
```

This also runs `go-engine/migrations/*.sql` automatically on first startup of a fresh volume. If you ever need to re-run migrations from scratch:

```powershell
docker compose down -v
docker compose up -d postgres redis
```

### 2. Configure environment files

Each service ships a `.env.example` — copy it to `.env` (or `.env.local` for the frontend) and fill in real values. **`INTERNAL_API_SECRET` must be the exact same string in all three files.**

```powershell
cd go-engine
Copy-Item .env.example .env
notepad .env
```

```powershell
cd ..\py-planner
Copy-Item .env.example .env
notepad .env
```

```powershell
cd ..\frontend
Copy-Item .env.local.example .env.local
notepad .env.local
```

In `py-planner\.env`, set `DEFAULT_LLM_PROVIDER=gemini` if you're using the free Gemini tier rather than Anthropic (see **LLM provider notes** below).

### 3. Install dependencies

```powershell
cd go-engine
go mod tidy

cd ..\mock-api
go mod tidy

cd ..\py-planner
pip install --break-system-packages -r requirements.txt

cd ..\frontend
npm install
```

### 4. Run all four services (separate terminals)

```powershell
# Terminal 1
cd go-engine
go run ./cmd/server

# Terminal 2
cd py-planner
uvicorn app.main:app --reload

# Terminal 3
cd mock-api
go run .

# Terminal 4
cd frontend
npm run dev
```

### 5. Seed the demo tenant

Once all three backend services are running, in a fifth terminal:

```powershell
cd go-engine
$env:INTERNAL_API_SECRET = "your-shared-secret"
go run ./cmd/seed
```

This creates one demo tenant, registers `mock-api`'s tools (parsed live from its OpenAPI spec), and registers a small hand-picked Stripe Customers tool set. It prints a tenant UUID — copy it.

### 6. Use it

Open `http://localhost:3000`, paste the tenant UUID into **Set tenant**, and try:

```
list all products
```

or, to see the confirmation gate in action:

```
place an order for 2 wireless mice
```

## LLM provider notes

`py-planner` supports both Anthropic and Gemini through raw function-calling (`DEFAULT_LLM_PROVIDER` in `.env`). A few things worth knowing if you're running this fresh:

- **Model names go stale.** Both providers rotate and deprecate model strings. If you get a 404 on a hardcoded model name, check the provider's current model list rather than assuming the one in this repo is still valid.
- **Gemini's free tier changes which models are available.** `gemini-flash-latest` is used here specifically because it's an auto-updated alias Google keeps pointed at whatever current Flash model is live — it's more resistant to this problem than pinning an exact version.
- **Gemini 3.x requires `thought_signature` replay on function calls.** If you see `400 Function call is missing a thought_signature`, it means the SDK/version in use predates this requirement. This repo uses the `google-genai` SDK (not the older `google-generativeai`) specifically to handle this correctly — the Gemini path in `agent_loop.py` maintains its own native `types.Content` history and replays the model's own response objects verbatim, which is what preserves the signature. The Anthropic path is unaffected since Claude has no equivalent requirement.
- Across a destructive-action confirmation pause (the browser round-trip while a human approves/rejects), Gemini's signed history can't be reconstructed from storage, so the agent loop recaps prior steps as plain text instead of replaying raw function-call parts. This avoids the 400 entirely at a small cost to conversational fidelity right after a confirmation.

## Known MVP limitations

- The frontend calls `go-engine` directly from the browser using a shared secret exposed via `NEXT_PUBLIC_*`. Fine for a single demo tenant; before any real deployment, move these calls behind a Next.js Route Handler and keep the secret server-side only.
- Auth is a single shared secret (`INTERNAL_API_SECRET`), not per-tenant credentials — this is a demo-scale simplification.
- The WebSocket trace stream isn't behind the internal-auth middleware (browsers can't attach custom headers during the WS handshake); `run_id` acts as the de facto bearer token. Add a signed short-lived ticket before any multi-tenant deployment.
- Stripe tools are hand-curated (a handful of Customers endpoints), not parsed from Stripe's full OpenAPI spec, which has thousands of operations.

## GitHub

github.com/MuaazTasawar/integradock