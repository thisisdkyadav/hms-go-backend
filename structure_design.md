# Go Backend Structure Design

This document defines the design rules for the Go backend. The goal is simple code that still holds up when the app becomes very large.

## Rule 1: Keep One Clear Layer Boundary

Use this order:

1. handler: HTTP-specific work only
2. service: business rules and orchestration
3. repository: persistence and query details

Avoid:

1. handlers calling Mongo or Redis directly
2. repositories containing response formatting
3. services writing raw HTTP responses

## Rule 2: Start Small, But Do Not Collapse Responsibilities

A module can stay in a few files if the scope is small.

Split only when needed, but keep the three responsibilities distinct:

- transport
- domain logic
- persistence

This is the Go equivalent of the backend guidance already used in Node.

## Rule 3: One Shared Response and Error Style

Every route should use the same envelope and the same error path.

Rules:

1. decode input in one place
2. return typed application errors from services
3. map errors to the HTTP envelope centrally
4. never hand-build ad-hoc JSON shapes per route

## Rule 4: Shared Helpers Must Stay Truly Shared

Put code in `internal/shared` only when it is reused by multiple modules or clearly belongs to platform behavior.

Do not move auth-specific helpers into `internal/shared` just because they are convenient.

## Rule 5: Infrastructure Adapters Stay Thin

Mongo and Redis packages should:

- establish connections
- expose clients
- stay free of domain rules

Repositories own query behavior. Platform packages own connection behavior.

## Rule 6: Prefer Explicit Contracts Over Framework Magic

Choose code that a teammate unfamiliar with Go can still follow quickly.

That means:

- small structs
- explicit constructors
- explicit route registration
- explicit middleware wrapping

Avoid deep reflection-driven patterns and hidden dependency injection containers.

## Rule 7: Migration Discipline

When extracting from Node:

1. preserve the frontend contract first
2. keep Node and Go ownership boundaries explicit
3. document temporary limitations honestly
4. handle session interoperability as a separate migration step

Do not smuggle cross-service coupling into random helpers.
