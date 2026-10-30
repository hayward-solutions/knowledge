# A3 repo-infra notes

## Created or edited

- docker-compose.yml: db, minio, minio-createbucket, api, web. Api has `read_only: true`, tmpfs `/tmp`, healthcheck `["CMD","/app/server","healthcheck"]` every 5s, build arg `VERSION`. Web has build args `NEXT_PUBLIC_API_BASE_URL` and `VERSION`, depends on api healthy. No `STORAGE_S3_PUBLIC_ENDPOINT`.
- .env.example, .gitignore, .editorconfig
- CLAUDE.md, README.md
- docs/CONFIGURATION.md and docs/DEVELOPMENT.md (real content); docs/API.md and docs/DEPLOYMENT.md (placeholders)
- .github/workflows/pull_request.yaml, .github/workflows/merge.yaml
- .claude/settings.local.json: existing Read allows kept; added go, npm, npx, docker, curl (localhost:8080 and localhost:3000 URL patterns, no credentials), and read-only shell allows. No git or gh entries.

## Decisions and deviations

- `SESSION_SECRET` is omitted from compose and .env.example. The brief's api env list does not include it and Backlog's docs call it reserved and unused. If the api config requires it, add it to compose, .env.example, and CONFIGURATION.md.
- `API_PORT`, `STORAGE_S3_REGION`, `STORAGE_S3_ENDPOINT`, `STORAGE_S3_FORCE_PATH_STYLE` are fixed literals in compose (not overridable from .env) so the port mapping and MinIO wiring cannot drift. Override them only when running the api outside compose.
- `VERSION` compose build arg defaults to `dev` and can be set from the shell or .env.
- `NEXT_PUBLIC_API_BASE_URL` is not set as a web runtime env var; it is a build arg only, since the standalone image bakes it in.
- OIDC defaults match Backlog's compose (scopes and admin group empty, claims groups/email/name, provider name SSO). The .env.example OIDC block keeps only the commented-out variable lines; Backlog's prose comments are in CONFIGURATION.md instead.
- No comments in compose, workflows, .gitignore, or .editorconfig.
- pull_request.yaml uses `npm run typecheck`, so web/package.json must define a `typecheck` script (plan text says `npx tsc --noEmit`; the brief says `npm run typecheck`).
- merge.yaml keeps Backlog's structure; the unused `cli` and `helm` filter outputs were dropped. `publish-manifests` is skipped when `build` is skipped, as in Backlog.
- minio healthcheck uses `curl` inside the `minio/minio:latest` image exactly as Backlog does. If a newer MinIO image drops curl, switch to `mc ready local`.
- The existing .git directory was not touched; no git commands were run.

## Gate output

```
cp .env.example .env && docker compose config   -> exit 0 (.env deleted afterwards)
docker compose config without .env              -> exit 0
yaml.safe_load pull_request.yaml                -> ok
yaml.safe_load merge.yaml                       -> ok
yaml.safe_load docker-compose.yml               -> ok
json load .claude/settings.local.json           -> ok
```

`docker compose config` rendered `read_only: true`, the tmpfs, the `/app/server healthcheck` test, `COOKIE_SECURE: "false"`, `ATTACHMENT_MAX_BYTES: "26214400"`, and `VERSION: dev` for both builds.
