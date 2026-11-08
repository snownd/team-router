# Repository Guidelines

## Project Structure & Module Organization
`team-router` is a Go 1.23 LLM API gateway. `cmd/gateway` is the Uber FX entry point. Domain code lives in `internal/`: `protocol` for wire/canonical conversion, `provider` and `relay` for routing/upstream calls, `admin`, `auth`, `billing`, `usage`, `repository`, and related infrastructure. GORM models and PostgreSQL helpers are under `internal/model` and `internal/database`. Versioned SQL migrations are in `migrations`; architecture and extension guides are in `docs`. Deployment assets are in `deploy` and `Dockerfile`.

## Build, Test, and Development Commands
- `go run ./cmd/gateway`: start locally. Copy `config.example.yaml` to `config.yaml` first, or set the `TR_*` variables in `.env.example`.
- `go test ./...`: run unit tests. Database-backed tests require `TEST_PG_DSN`.
- `go test ./internal/protocol`: run the focused protocol conversion tests.
- `go build ./cmd/gateway`: compile the gateway.
- `cd deploy && docker compose up -d --build`: start PostgreSQL plus the gateway with migrations and seed data.

## Coding Style & Naming Conventions
Use standard Go formatting: `gofmt`/`goimports`, tabs for indentation, exported identifiers documented with comments where useful. Keep package boundaries intentional; add handlers, adapters, repositories, and models in their existing packages rather than creating cross-package shortcuts. Name Go files in snake_case, tests as `<source>_test.go`, tests as `Test<Behavior>`, and migration files as sequential `NNNN_description.up.sql` / `.down.sql`.
### 1. Anti-Defensive Programming

- Before adding complexity for a speculative or vanishingly unlikely race or security edge case, explain the concrete failure mode, likelihood, and complexity cost to the user and get their buy-in.

- Do not silently expand scope for theoretical robustness.

- If you claim an edge case or race condition exists, you MUST provide the exact sequence of events and write a failing reproducible test BEFORE introducing any synchronization, locking, or defensive layers. Without a reproducible test, do not add defensive code.



### 2. Keep It Simple

- Keep logic in a single function unless it is genuinely reusable or composable.

- Do not extract single-use helpers preemptively. Inline the logic at the call site unless the helper is reused, hides a genuinely complex boundary, or has a clear independent name that improves the caller.

- Validate unknown values once at the boundary that owns them. Pass typed values inward; do not defensively revalidate values already guaranteed by a schema, constructor, or internal type.

## Testing Guidelines
Use Go's standard testing package. Unit-test protocol, billing, auth, routing, and usage logic; use `internal/database/dbtest` with PostgreSQL for repository and partition tests. Prefer realistic wire/SSE fixtures for protocol round trips. Run `go test ./...` with a PostgreSQL DSN before submitting database-affecting changes.

## Commit & Pull Request Guidelines
Git metadata was unavailable in this workspace, so no historical convention could be verified. Use concise imperative subjects such as `feat: add Gemini adapter` or `fix: preserve stream usage`. Pull requests should state the user-visible change, implementation approach, test evidence, linked issue, and migration/deployment impact. Include request/response examples or screenshots for API/UI behavior.

## Security & Configuration Tips
Never commit `.env`, `config.yaml`, API keys, or database credentials. In production, set a strong `TR_AUTH_JWT_SECRET` and a 32-byte hex `TR_CRYPTO_MASTER_KEY`. Preserve AES-GCM encryption of upstream provider keys and avoid logging secrets or bearer tokens.
