# HMS Go Backend

This workspace is the beginning of the Go migration for HMS.

Current scope:

- own authentication and session flows
- preserve the frontend-facing auth HTTP contract
- stay small enough to reason about
- stay structured enough to scale into a full backend over time

What is included:

- a production-oriented Go service layout
- shared config, Mongo, Redis, HTTP, session, and email plumbing
- an `auth` module covering login, logout, session refresh, SSO, Google token login, password reset, pinned tabs, and device session management
- structure documents that define how new Go modules should be added
- migration notes that keep the Node-to-Go boundary explicit

What is intentionally not done yet:

- frontend changes
- routing production traffic from Node to Go
- non-auth domain migration

## Folder Map

```text
go-backend/
├── cmd/api/                  # executable entrypoint
├── internal/app/             # service bootstrap and dependency wiring
├── internal/config/          # environment loading and config models
├── internal/platform/        # Mongo and Redis adapters
├── internal/shared/          # reusable HTTP/session/email helpers
├── internal/modules/
│   └── auth/                 # auth domain
├── STRUCTURE_GUIDE.md        # source of truth for folder ownership
└── structure_design.md       # design rules for future Go work
```

## Run

1. Copy `.env.example` to `.env`.
2. Point the service at the existing MongoDB and Redis.
3. Run `go run ./cmd/api`.

## Cookie Defaults

- use `SESSION_SECURE=true` and `SESSION_SAME_SITE=none` when the app is served over HTTPS
- use `SESSION_SECURE=false` and `SESSION_SAME_SITE=lax` for plain HTTP environments

## Migration Note

This service is phase 1 of the move from Node to Go. Auth now writes Express-compatible Redis sessions so the existing Node backend can continue authenticating requests after login happens in Go. The remaining migration work is still domain-by-domain.
