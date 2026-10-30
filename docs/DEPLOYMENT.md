# Deployment

This document covers the published container images, the runtime constraints they are built for, and the layout of the Kubernetes deployment on the pi-lab k3s cluster, where Knowledge runs beside Backlog. The pi-lab Terraform stack for Knowledge is not written yet; the checklist at the end lists what it needs, and Backlog's stack is the template.

Backlog's stack is at `pi-lab/terraform/001-business-tools/backlog/` (`namespace.tf`, `postgres.tf`, `bucket.tf`, `oidc.tf`, `secrets.tf`, `deployment-api.tf`, `deployment-web.tf`, `ingress.tf`). Copy it and change what is listed below. See [CONFIGURATION.md](CONFIGURATION.md) for what each variable means.

---

## Images

`.github/workflows/merge.yaml` runs on every push to `main` that changes `api/**`, `web/**`, or the workflow itself. It builds each service on its native runner, so there is no emulation: `amd64` on `ubuntu-latest` and `arm64` on `ubuntu-24.04-arm`.

For each of `api` and `web` it pushes:

| Tag | Content |
|---|---|
| `ghcr.io/hayward-solutions/knowledge/{api,web}:main-amd64` | Latest `main`, amd64. |
| `ghcr.io/hayward-solutions/knowledge/{api,web}:main-arm64` | Latest `main`, arm64. This is what the cluster pulls. |
| `ghcr.io/hayward-solutions/knowledge/{api,web}:<sha>-amd64` and `:<sha>-arm64` | The commit's full SHA, per architecture. |
| `ghcr.io/hayward-solutions/knowledge/{api,web}:main` | Multi-arch manifest of the two `main-*` tags, created by the `publish-manifests` job. |
| `ghcr.io/hayward-solutions/knowledge/{api,web}:<sha>` | Multi-arch manifest of the two SHA tags. Use this to pin a release. |

The build context is `api/` or `web/`. Both images take a `VERSION` build argument, set to `main-<first 8 characters of the SHA>` in CI. It appears at `GET /health` and in the account menu. The CI build passes no `NEXT_PUBLIC_API_BASE_URL`, so the published web image calls the API on its own origin (`/api/v1/...`); see [Ingress](#ingress).

Pull requests do not build images; they run the lint, typecheck, build, and test jobs from [DEVELOPMENT.md](DEVELOPMENT.md#ci). To build locally:

```bash
docker build --build-arg VERSION=local -t knowledge-api:local api
docker build --build-arg VERSION=local --build-arg NEXT_PUBLIC_API_BASE_URL= -t knowledge-web:local web
```

## Runtime constraints

The images are built to run unprivileged with a read-only root filesystem, as the cluster runs them.

### API (`knowledge/api`)

| | |
|---|---|
| Base image | `gcr.io/distroless/static-debian12:nonroot` (no shell, no `wget`) |
| User | uid and gid `65532` (`nonroot`) |
| Root filesystem | Read-only works. `/tmp` must be writable: uploads are spooled to a temp file there. Use tmpfs or an `emptyDir`. |
| Port | `8080` (`API_PORT`) |
| Build | `golang:1.26-alpine` builder, static binary |
| Entrypoint | `/app/server`; migrations are in `/app/migrations` (`MIGRATIONS_DIR` is set in the image) |
| Timeouts | Header read 10 s, request read 60 s (5 min for attachment and import uploads), idle 120 s, no write timeout (SSE). Postgres statements are cut at 30 s. |
| Liveness | `GET /health` on 8080 returns `{"ok":true,"version":"..."}`. The exec form `/app/server healthcheck` does the same without a shell (used by Compose). |
| Shutdown | `SIGTERM` starts a graceful shutdown with a 10 second timeout. |
| Replicas | Exactly 1. The event bus, SSE hub, and presence tracker are in memory (see [DEVELOPMENT.md](DEVELOPMENT.md#events-bus-and-sse)). |

On every start the API waits up to 30 seconds for Postgres, runs the goose migrations, bootstraps the first admin if the `users` table is empty, seeds the starter templates if there are none, and then listens. Database access, object storage, and the OIDC issuer (discovery runs at start) must be reachable from the pod, or the API exits.

### Web (`knowledge/web`)

| | |
|---|---|
| Base image | `node:22-alpine`, Next.js `output: "standalone"` |
| User | uid and gid `1000` (`node`) |
| Root filesystem | `/app/.next/cache` must be writable. The image creates it owned by `node`; mount an `emptyDir` there if the root filesystem is read-only. |
| Port | `3000` (`HOSTNAME=0.0.0.0`) |
| Command | `node server.js` |
| Runtime config | None. `NEXT_PUBLIC_*` values are baked in at build time. |
| Probe | The image defines none. `GET /login` answers `200` without a session; Compose's healthcheck is `wget -q -O /dev/null http://127.0.0.1:3000/login`, and Backlog's deployment defines no probe for web. |

The web container never talks to Postgres or S3; it serves the app, and the browser calls the API.

## Ingress

On the cluster, one Tailscale ingress serves a single hostname, `https://knowledge.<tailnet>.ts.net`, and splits traffic by path:

| Path | Backend |
|---|---|
| `/api` | `knowledge-api:8080` |
| `/health` | `knowledge-api:8080` |
| everything else | `knowledge-web:3000` (the default backend) |

Because the browser reaches the API on the same origin, the web image is built with `NEXT_PUBLIC_API_BASE_URL` **empty** (the CI default), and no CORS is involved. The matching API settings:

| Var | Value |
|---|---|
| `PUBLIC_BASE_URL` | `https://knowledge.<tailnet>.ts.net` |
| `APP_ORIGINS` | `https://knowledge.<tailnet>.ts.net` |
| `COOKIE_SECURE` | `true`, since the ingress terminates TLS |
| `TRUSTED_PROXIES` | The address range of the pods that run the Tailscale ingress proxy (see below) |
| `OIDC_REDIRECT_URL` | `https://knowledge.<tailnet>.ts.net/api/v1/auth/oidc/callback` |

**`TRUSTED_PROXIES`.** The API only sees the ingress proxy's pod address as the TCP peer. With `TRUSTED_PROXIES` empty (the default) it ignores `X-Forwarded-For`, so every user shares one login rate-limit bucket and request logs show the proxy. Set it to the proxy's range so the API reads the real client from `X-Forwarded-For`. Find the address with `kubectl get pods -n tailscale -o wide` (the pod for the `knowledge` ingress), or use the cluster pod CIDR (k3s defaults to `10.42.0.0/16`; confirm yours). Trust only that range: anything inside it can claim any client IP. After deploying, confirm it took effect by checking that the client address in the API log or the per-IP behaviour is the caller's tailnet address, not the proxy's; if the proxy does not send `X-Forwarded-For`, the limit stays keyed on the proxy.

The SSE stream at `/api/v1/notifications/stream` goes through the same ingress; after deploying, confirm in the browser's network panel that it stays open and that a notification arrives without a reload. Attachments are proxied by the API, so the browser never needs to reach RustFS, only the API pod does.

## Postgres and object storage

**Postgres.** The pi-lab `postgres` module runs `postgres:15` on a Longhorn volume; Compose uses 16. Knowledge works on both: the API test suites pass against 15. The role the API connects as must be able to run `CREATE EXTENSION IF NOT EXISTS citext` (migration `0001`): a superuser, which the image's `POSTGRES_USER` is. The DSN is `postgres://<user>:<password>@postgres:5432/<db>?sslmode=disable`, with the host being the service name in the namespace.

**Object storage (RustFS).** RustFS is the S3 API at `https://rustfs.<tailnet>.ts.net`. Clients must use **path-style** addressing and region **`us-east-1`**:

| Var | Value |
|---|---|
| `STORAGE_S3_BUCKET` | the bucket created by the `rustfs-bucket` module |
| `STORAGE_S3_REGION` | `us-east-1` |
| `STORAGE_S3_ENDPOINT` | the module's `endpoint` output |
| `STORAGE_S3_FORCE_PATH_STYLE` | `true` |
| `AWS_ACCESS_KEY_ID` | the module's `access_key` (a user scoped to that one bucket) |
| `AWS_SECRET_ACCESS_KEY` | the module's `secret_key` |

There is no public storage endpoint and no bucket CORS; only the API pod needs a route to RustFS. Because the namespace denies egress by default, that route is the namespace's `tailscale_ingresses` egress rule plus an entry for the namespace in the RustFS ingress `cluster_access` (both in the checklist).

## Secrets

Values that must be provided to the API, and where Backlog's pattern puts them:

| Variable | Source |
|---|---|
| `POSTGRES_DSN` | Written by hand into `secret/knowledge/manual`. It contains the database password, which is also in `secret/knowledge/postgres-password`; if the password changes, update both. |
| `ADMIN_PASSWORD` | By hand into `secret/knowledge/manual`. Must meet the password policy. Only used while the `users` table is empty. |
| `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` | Written by Terraform from the Cognito client module. |
| `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | Written by Terraform from the `rustfs-bucket` module. |

Non-secret settings go in the deployment's `env` block: `ADMIN_EMAIL`, `ADMIN_DISPLAY_NAME`, `API_PORT`, `PUBLIC_BASE_URL`, `APP_ORIGINS`, `COOKIE_SECURE`, `TRUSTED_PROXIES`, `OIDC_ISSUER_URL`, `OIDC_REDIRECT_URL`, `OIDC_SCOPES`, `OIDC_GROUPS_CLAIM`, `OIDC_ADMIN_GROUP`, `OIDC_PROVIDER_NAME`, `STORAGE_S3_*`, and optionally `ATTACHMENT_MAX_BYTES` and `OIDC_REQUIRE_EMAIL_VERIFIED` (default `true`, which Cognito satisfies by sending `email_verified`). For Cognito, Backlog uses `OIDC_SCOPES=openid,email,profile` (Cognito rejects the default `groups` scope), `OIDC_GROUPS_CLAIM=cognito:groups`, and `OIDC_ADMIN_GROUP` from the client module's `admin_group` output. The pi-lab secret module merges the Terraform-written path and the hand-written path into one Secret, loaded with `env_from`.

## Checklist for the pi-lab stack

Create `terraform/001-business-tools/knowledge/` modelled on the `backlog` stack, with `app_name = "knowledge"`, so services are `knowledge-api` and `knowledge-web`. Each item names the Backlog file to copy.

1. **Backend and providers** (`backend.tf`, `providers.tf`, `variables.tf`): copy them, change the state key to `knowledge.tfstate`, and set `app_name`, `domain`, `admin_email`, and `admin_display_name`.
2. **Namespace** (`namespace.tf`): the `namespace` module with `name = var.app_name` and `network_policy.egress.tailscale_ingresses = [{ name = "rustfs", namespace = "rustfs" }]`, so the API can reach the RustFS ingress. If the Knowledge images are private on GHCR, also set `ghcr = true` and use `module.namespace.image_pull_secret_name` as the pull secret; Backlog sets neither, so its images are pulled without credentials.
3. **RustFS ingress** (`terraform/000-platform/rustfs/ingress.tf`): add `"knowledge"` to `cluster_access.namespaces` (currently `backlog`, `docshare`, `sealskin`, `vincenty`) and apply the `rustfs` stack. Without this, pods in the Knowledge namespace cannot resolve or reach RustFS's tailnet name.
4. **Bucket** (`bucket.tf`): the `rustfs-bucket` module with a bucket name such as `knowledge-assets` and `kube_namespace = module.namespace.name`.
5. **Postgres** (`postgres.tf`): the `postgres` module with the namespace's `secret_store`. Its password is synced from OpenBao `secret/knowledge/postgres-password` (key `password`), which you write by hand **before** the first apply; Postgres reads it only when its data directory is first created. Set `db_name` and `db_user` if you do not want the defaults.
6. **OIDC** (`oidc.tf`): the Cognito `client` module with `app_name = "harptree-knowledge"`, `callback_urls = ["https://knowledge.<domain>/api/v1/auth/oidc/callback"]`, and `logout_urls = ["https://knowledge.<domain>/login"]`.
7. **Secrets** (`secrets.tf`): the `secret` module with `name = "secrets"`, `write_values = true`, `extra_paths = ["manual"]`, and `values` of `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `AWS_ACCESS_KEY_ID`, and `AWS_SECRET_ACCESS_KEY`. Then write `POSTGRES_DSN` and `ADMIN_PASSWORD` by hand to `secret/knowledge/manual` in OpenBao. The synced Secret is `knowledge-secrets`.
8. **API deployment** (`deployment-api.tf`): one replica of `ghcr.io/hayward-solutions/knowledge/api:main-arm64` (`image_pull_policy = "Always"`). Pod security context `run_as_non_root`, uid and gid `65532`, seccomp `RuntimeDefault`. Container security context with `read_only_root_filesystem = true`, `allow_privilege_escalation = false`, and all capabilities dropped. An `emptyDir` mounted at `/tmp`. Port 8080 and an HTTP liveness probe on `/health`. The non-secret `env` above, with `COOKIE_SECURE = "true"` and `TRUSTED_PROXIES` set to the Tailscale proxy range (neither has an equivalent in Backlog's list), and `env_from` the `knowledge-secrets` Secret. A `knowledge-api` Service on 8080.
9. **Web deployment** (`deployment-web.tf`): one replica of `ghcr.io/hayward-solutions/knowledge/web:main-arm64`, uid and gid `1000`, an `emptyDir` mounted at `/app/.next/cache`, port 3000, and a `knowledge-web` Service on 3000. No environment variables.
10. **Ingress** (`ingress.tf`): the `tailscale-ingress` module with `name = "knowledge"`, `target_service = "knowledge-web"`, `target_port = "3000"`, and `paths` for `/api` and `/health`, both to `knowledge-api` on port 8080. Add the hub annotations: `hub.hayward.solutions/enabled = "true"`, `hub.hayward.solutions/category = "business"`, `hub.hayward.solutions/name = "Knowledge"`, and a `hub.hayward.solutions/description` such as `Documentation and knowledge sharing`.
11. **Repo docs**: add `knowledge/secrets` plus `knowledge/manual` and the `knowledge/postgres-password` row to the "What's stored" table in `terraform/000-platform/openbao/README.md`, and list the stack under `001-business-tools` in the pi-lab `README.md`.
12. **After apply**: `curl https://knowledge.<domain>/health` returns the version; sign in as the bootstrap admin (and through Cognito); create a space, publish a page, upload an attachment, and confirm a notification arrives live. Then work through the [verification walkthrough](CONFIGURATION.md#verification-walkthrough).

Roll out a new build by restarting the deployments, since `main-arm64` is a moving tag: `kubectl rollout restart deployment/knowledge-api deployment/knowledge-web -n knowledge`. Migrations run when the API starts.

Everything stateful lives in Postgres and the RustFS bucket; neither pod keeps data on its filesystem, so back up those two.
