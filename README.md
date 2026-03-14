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
- cross-service session interoperability with the existing Node backend
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

## Migration Note

This service is phase 1 of the move from Node to Go. It mirrors the current auth contract cleanly, but the rest of the HMS backend still assumes Node-owned sessions. That interoperability work should be handled as a dedicated next step instead of being hidden inside this scaffold.
