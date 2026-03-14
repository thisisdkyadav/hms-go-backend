# Go Auth Migration Notes

This Go backend is phase 1 of the backend migration.

## What This Phase Does

- creates a new Go service focused on authentication
- keeps the auth HTTP contract aligned with the current frontend
- introduces the permanent Go backend structure and shared conventions

## What This Phase Does Not Do Yet

- replace the Node backend in production
- share session state with the Node backend
- move authz, complaints, students, or other domains into Go

## Important Boundary

Right now, Go owns auth behavior inside `go-backend`, but the existing Node backend still owns the rest of the application and still expects Node-created sessions.

That means one follow-up decision is unavoidable:

1. either make Node and Go share one session contract in Redis
2. or route all authenticated traffic through Go as more domains migrate

This scaffold intentionally does not hide that decision inside temporary hacks.

## Recommended Next Step

When we continue, the next task should be one of these:

1. define a shared Redis session contract that both Node and Go can read
2. put the Go auth service behind the same gateway/proxy path as `/api/v1/auth`
3. move the authz endpoints next, so auth and authz stop being split across runtimes
