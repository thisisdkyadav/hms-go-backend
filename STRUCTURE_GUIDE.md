# Go Backend Structure Guide

Purpose: define the permanent structure for HMS Go services.
Last updated: March 14, 2026

## 1. Architecture Overview

The Go backend should stay a modular monolith for as long as possible.

- Shared runtime and infrastructure belong in `internal/config`, `internal/platform`, and `internal/shared`.
- Business logic belongs in `internal/modules/<domain>`.
- `cmd/api` is the only executable entrypoint for the HTTP service.

This keeps the codebase small enough for one team to understand while still supporting large app growth.

## 2. Current Directory Layout

```text
go-backend/
├── cmd/
│   └── api/
├── internal/
│   ├── app/
│   ├── config/
│   ├── platform/
│   │   ├── mongo/
│   │   └── redis/
│   ├── shared/
│   │   ├── email/
│   │   ├── httpx/
│   │   └── session/
│   └── modules/
│       └── auth/
├── README.md
├── STRUCTURE_GUIDE.md
└── structure_design.md
```

## 3. Folder Ownership Rules

### 3.1 `cmd/api`

- Owns process startup only.
- No business logic.
- No database queries.

### 3.2 `internal/app`

- Wires dependencies together.
- Creates the HTTP server.
- Registers modules.
- Owns graceful shutdown.

### 3.3 `internal/config`

- Loads and validates environment configuration.
- Exposes typed config objects only.
- No domain logic.

### 3.4 `internal/platform`

- Owns third-party infrastructure adapters.
- Mongo and Redis connection setup belongs here.
- These packages should stay thin and mechanical.

### 3.5 `internal/shared`

- Owns reusable helpers used by 2+ modules.
- Good examples: response envelopes, JSON decoding, session manager, SMTP sender.
- Do not move domain-specific logic here.

### 3.6 `internal/modules/<domain>`

Each domain owns its own transport, service, repository, and models.

For auth:

- HTTP handlers live in `internal/modules/auth`
- auth business rules live in `internal/modules/auth`
- auth data access stays in `internal/modules/auth`

Future modules should follow the same ownership pattern.

## 4. HTTP Contract Rules

Every Go route should use one standard response envelope:

```json
{
  "success": true,
  "message": null,
  "data": {},
  "errors": null
}
```

On error:

```json
{
  "success": false,
  "message": "Request failed",
  "data": null,
  "errors": []
}
```

Rules:

- controllers/handlers do input decoding and call services
- services return data plus intentional domain errors
- response formatting stays centralized

## 5. Add New Work

### 5.1 Add to an Existing Module

1. Keep the new capability inside the owning module.
2. Reuse shared helpers only when the helper is already cross-domain.
3. Do not create a new top-level folder for a sub-feature.

### 5.2 Add a New Module

1. Create `internal/modules/<domain>/`.
2. Add:
   - route registration
   - handler
   - service
   - repository
   - local models and DTOs as needed
3. Register the module in `internal/app`.

## 6. Migration Guidance

The Go backend will grow by domain extraction from Node.

Rules:

- migrate one domain at a time
- preserve frontend contracts unless the migration explicitly includes frontend updates
- keep shared operational concerns centralized
- do not mix transport and database code directly in handlers

## 7. Verification Checklist

Before merging Go changes:

- `go test ./...` passes
- routes still use the standard response envelope
- new config is documented in `.env.example`
- module ownership is still obvious from folder structure
