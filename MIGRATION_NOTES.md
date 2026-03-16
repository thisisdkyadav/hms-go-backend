# Go Auth Migration Notes

This Go backend is phase 1 of the backend migration.

## What This Phase Does

- creates a new Go service focused on authentication
- keeps the auth HTTP contract aligned with the current frontend
- writes Express-compatible Redis sessions so Node can still authenticate users
- introduces the permanent Go backend structure and shared conventions

## What This Phase Does Not Do Yet

- replace the Node backend in production
- move authz, complaints, students, or other domains into Go

## Important Boundary

Right now, Go owns auth behavior inside `go-backend`, but the existing Node backend still owns the rest of the application.

The session bridge is now handled by keeping these aligned between Go and Node:

1. `REDIS_URL`
2. `REDIS_SESSION_PREFIX`
3. `SESSION_SECRET`
4. cookie name (`connect.sid`)
5. session TTL expectations

That lets Go-authenticated users continue into Node-owned routes without re-login.

## Recommended Next Step

When we continue, the next task should be one of these:

1. put the Go auth service behind the same gateway/proxy path as `/api/v1/auth`
2. move the authz endpoints next, so auth and authz stop being split across runtimes
3. migrate the next backend domain while keeping the shared session contract stable
