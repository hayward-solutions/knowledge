# Configuration

This document covers every environment variable, the storage backend, OIDC setup, the role and visibility model, and how to recover a lost admin. See [DEVELOPMENT.md](DEVELOPMENT.md) for running without Docker, [DEPLOYMENT.md](DEPLOYMENT.md) for the published images and the cluster layout, and [API.md](API.md) for the HTTP surface.

## Architecture

```
┌───────────────┐   ┌────────────────┐   ┌──────────────┐
│ Next.js (web) │◄─►│  Go API (chi)  │◄─►│ Postgres 16  │
│  :3000        │SSE│  :8080         │   │  :5432       │
└───────────────┘   └────────┬───────┘   └──────────────┘
                             │
                             ▼
                    ┌────────────────┐
                    │ S3 (RustFS)    │
                    │  :9000/:9001   │
                    └────────────────┘
```

All five containers ship in the bundled `docker-compose.yml`: `db`, `rustfs`, `rustfs-createbucket` (a one-shot job), `api`, and `web`.

Attachments are proxied through the API in both directions. The browser never talks to object storage, so no public storage endpoint and no CORS configuration on the bucket are needed.

---

## Environment variables

Configuration is read from the `.env` file consumed by `docker-compose.yml`; every variable has a default in the compose file, so `.env` only needs the values you change. `.env.example` is a copy-pasteable starting point. Variable names are identical to Backlog's, plus `COOKIE_SECURE`, `ATTACHMENT_MAX_BYTES`, `TRUSTED_PROXIES`, and `OIDC_REQUIRE_EMAIL_VERIFIED`.

`docker-compose.yml` forwards only the variables in the tables below that have a Compose default; `TRUSTED_PROXIES` and `OIDC_REQUIRE_EMAIL_VERIFIED` are not forwarded, so to use them with Compose add them to the `api` service's `environment` block. Outside Compose, export them like any other variable.

Two defaults are listed per variable. **Code default** is what the API uses when the variable is unset (for example when you run it outside Compose). **Compose default** is what `docker-compose.yml` passes when `.env` does not set it.

### Database

| Var | Purpose | Code default | Compose default |
|---|---|---|---|
| `POSTGRES_DSN` | API to Postgres connection string. Required: the API exits without it. | none | `postgres://knowledge:knowledge@db:5432/knowledge?sslmode=disable` |
| `POSTGRES_DB` | Database name created by the `db` service. Compose only. | n/a | `knowledge` |
| `POSTGRES_USER` | Database user created by the `db` service. Compose only. | n/a | `knowledge` |
| `POSTGRES_PASSWORD` | Database password set on the `db` service. Compose only; keep it in sync with the password in `POSTGRES_DSN`. | n/a | `knowledge` |

### API and web

| Var | Purpose | Code default | Compose default |
|---|---|---|---|
| `API_PORT` | API listen port. Also read by `server healthcheck`. | `8080` | fixed `8080` (not read from `.env`) |
| `MIGRATIONS_DIR` | Directory of goose migrations the API applies at start. The image sets `/app/migrations`. | `./migrations` | not set |
| `PUBLIC_BASE_URL` | Public URL of the web app (a trailing slash is trimmed). The OIDC callback redirects the browser here, so it is required for SSO. | empty | `http://localhost:3000` |
| `APP_ORIGINS` | Comma-separated CORS origin allowlist (exact origins). Only needed when the browser calls the API from a different origin than the page. | none | `http://localhost:3000` |
| `COOKIE_SECURE` | Set the `Secure` flag on the `sid` session cookie and the short-lived OIDC cookies. `true` behind HTTPS. | `false` | `false` |
| `TRUSTED_PROXIES` | Comma-separated IPs or CIDRs of the reverse proxies allowed to set `X-Forwarded-For` / `X-Real-IP`. Empty means those headers are never honoured and the TCP peer address is used for rate limiting and request logs. A bare IP means /32 (or /128). An invalid entry makes the API exit at start. See [Rate limiting and proxies](#rate-limiting-and-proxies). | empty | not forwarded |
| `NEXT_PUBLIC_API_BASE_URL` | Where the browser calls the API. A **web build argument**, baked into the JavaScript bundle and the Content-Security-Policy. Empty means same origin (`/api/v1/...`). | empty | `http://localhost:8080` |
| `VERSION` | Build argument for both images. Shown at `GET /health` (`version`) and in the account menu. | `dev` | `dev` |
| `ADMIN_EMAIL` | First-boot admin bootstrap email. | empty (bootstrap skipped) | `admin@example.com` |
| `ADMIN_PASSWORD` | First-boot admin bootstrap password. Must satisfy the password policy (below) or the API exits at start. | empty (bootstrap skipped) | `Change-Me-On-First-Boot-1` |
| `ADMIN_DISPLAY_NAME` | First-boot admin display name. | `Admin` | `Admin` |

Password policy, enforced for the bootstrap admin, admin-created users, password resets, and password changes: at least 12 characters, with an upper-case letter, a lower-case letter, a digit, and a symbol; at most 256 bytes. Passwords are stored as argon2id (m=32 MiB, t=3, p=2).

`NEXT_PUBLIC_API_BASE_URL` is read at **build** time. Changing it means rebuilding the web image. Behind a single hostname that routes `/api` to the API (see [DEPLOYMENT.md](DEPLOYMENT.md)), leave it empty.

### Attachment storage (S3)

The bundled compose file runs RustFS and a one-shot job that creates the bucket. Compose fixes the region, endpoint, and path style for it; set them yourself when running the API outside Compose or against another S3-compatible store.

| Var | Purpose | Code default | Compose default |
|---|---|---|---|
| `STORAGE_S3_BUCKET` | Bucket for attachments. **When unset, object storage is disabled** and attachment upload and download answer `503`. | none | `knowledge-dev` |
| `STORAGE_S3_REGION` | S3 region. | `us-east-1` | fixed `us-east-1` |
| `STORAGE_S3_ENDPOINT` | Endpoint the API talks to. Leave empty for AWS S3. | none | fixed `http://rustfs:9000` |
| `STORAGE_S3_FORCE_PATH_STYLE` | Path-style addressing (`endpoint/bucket/key`). `true` for RustFS, MinIO, and most S3-compatible stores. | `false` | fixed `true` |
| `AWS_ACCESS_KEY_ID` | Access key. Used with the secret key as static credentials when both are set; otherwise the AWS SDK default credential chain applies. | none | `minioadmin` |
| `AWS_SECRET_ACCESS_KEY` | Secret key. | none | `minioadmin` |
| `AWS_SESSION_TOKEN` | Optional session token for temporary credentials. | none | not set |
| `ATTACHMENT_MAX_BYTES` | Maximum upload size in bytes. Larger uploads answer `413`. Also caps each Markdown import file; the whole import body is capped at eight times this. | `26214400` (25 MiB) | `26214400` |
| `MINIO_ROOT_USER` | Root access key of the bundled object store container. Compose only. The name is historical (it matches Backlog's `.env`); compose passes it to RustFS as `RUSTFS_ACCESS_KEY`. | n/a | `minioadmin` |
| `MINIO_ROOT_PASSWORD` | Root secret key of the bundled container. Compose only. | n/a | `minioadmin` |

With the bundled store, `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` must equal `MINIO_ROOT_USER` and `MINIO_ROOT_PASSWORD`.

### OIDC single sign-on (optional)

Leave all `OIDC_*` unset to disable SSO and keep password-only login. See [OIDC setup](#oidc-setup).

| Var | Purpose | Code default | Compose default |
|---|---|---|---|
| `OIDC_ISSUER_URL` | Issuer URL. Provider discovery runs at API start. | none | empty |
| `OIDC_CLIENT_ID` | OAuth client id. | none | empty |
| `OIDC_CLIENT_SECRET` | OAuth client secret. | none | empty |
| `OIDC_REDIRECT_URL` | Callback URL registered with the IdP: the API's `/api/v1/auth/oidc/callback`, e.g. `http://localhost:8080/api/v1/auth/oidc/callback`. | none | empty |
| `OIDC_SCOPES` | Comma-separated scopes. | `openid,profile,email,groups` | empty (code default applies) |
| `OIDC_ADMIN_GROUP` | Group claim value that grants system admin. Empty disables group-to-admin mapping. | none | empty |
| `OIDC_GROUPS_CLAIM` | Claim carrying group membership. A string or an array of strings. | `groups` | `groups` |
| `OIDC_EMAIL_CLAIM` | Claim carrying the user's email. | `email` | `email` |
| `OIDC_NAME_CLAIM` | Claim carrying the display name. Falls back to the email. | `name` | `name` |
| `OIDC_PROVIDER_NAME` | Label on the "Sign in with ..." button. | `SSO` | `SSO` |
| `OIDC_REQUIRE_EMAIL_VERIFIED` | Only link an SSO login to an existing local account (matched by email) when the IdP says the email is verified. Accepts `1`, `true`, `yes`, `on` for true; any other non-empty value is false. | `true` | not forwarded |

If any of `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_REDIRECT_URL` is set, all four are required and the API exits at start otherwise. It also exits if the issuer cannot be reached for discovery.

---

## Storage

Attachments live in one S3 bucket, under the key `spaces/{spaceID}/pages/{pageID}/{attachmentID}/{filename}`.

- **Bundled store.** `docker compose up` starts RustFS (S3 API on `localhost:9000`, console at <http://localhost:9001/rustfs/console/>) and creates `STORAGE_S3_BUCKET` if it is missing. The same RustFS is what runs in the k3s cluster.
- **Path-style, region `us-east-1`.** RustFS needs path-style addressing and accepts region `us-east-1`. When an endpoint is set, the API's S3 client uses the "when required" checksum mode so non-AWS stores accept the requests.
- **Other S3-compatible stores.** MinIO, Ceph, and AWS S3 use the same variables (leave `STORAGE_S3_ENDPOINT` empty for AWS and unset path-style). Only RustFS is exercised by this repo's compose file and cluster.
- **No public endpoint.** There is no `STORAGE_S3_PUBLIC_ENDPOINT`. Uploads arrive as multipart at the API and are streamed to S3; downloads and inline images are streamed back by the API with the correct `Content-Type` and `Content-Disposition`. The bucket needs no public access and no CORS rules; only the API needs a network path to it.
- **Temp files.** Each upload is spooled once to a temporary file under `/tmp` to learn its size. A read-only root filesystem therefore needs a writable `/tmp` (Compose uses tmpfs; Kubernetes an `emptyDir`).
- **Cleanup.** Deleting an attachment removes its object. Purging a trashed page removes the objects of the page and its descendants. Soft-deleting a space leaves its objects in place.

---

## OIDC setup

1. In your IdP, register a confidential web application using the authorization code flow. Set the redirect URI to the API's callback: `https://knowledge.example.com/api/v1/auth/oidc/callback` behind a shared hostname, or `http://localhost:8080/api/v1/auth/oidc/callback` in local Compose.
2. Set `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, and `OIDC_REDIRECT_URL` to match, and set `PUBLIC_BASE_URL` to the public URL of the web app. Restart the API; it logs `oidc enabled (provider=...)`.
3. `GET /api/v1/auth/oidc/config` now returns `{"enabled": true, "provider_name": "..."}` and `/login` shows a **Sign in with ...** button. Password login keeps working.
4. Make sure the IdP puts the user's email in the ID token (or the userinfo endpoint), under `OIDC_EMAIL_CLAIM`. A login without an email is rejected.

Example for Cognito, which names its groups claim `cognito:groups` and does not accept a `groups` scope:

```bash
OIDC_ISSUER_URL=https://cognito-idp.eu-west-2.amazonaws.com/<pool-id>
OIDC_CLIENT_ID=<client id>
OIDC_CLIENT_SECRET=<client secret>
OIDC_REDIRECT_URL=https://knowledge.example.com/api/v1/auth/oidc/callback
OIDC_SCOPES=openid,email,profile
OIDC_GROUPS_CLAIM=cognito:groups
OIDC_ADMIN_GROUP=knowledge-admins
OIDC_PROVIDER_NAME=Cognito
```

### How accounts are provisioned

- A login is matched first by the IdP identity (`issuer` plus `sub`). If there is none, it is matched by **email** to an existing user, and the identity is linked to that user. Otherwise a new user is created with no local password.
- **Email verification.** Linking by email requires the ID token's `email_verified` claim to be `true` (a boolean, or the string `"true"` that Cognito sends). If the ID token has no `email_verified` key, the userinfo endpoint is asked. When the claim is false or missing, nothing is written (no identity, no session) and the callback answers `403` with a "Sign-in blocked" page that links back to `/login`. Signing in with an identity that is already linked, and creating a brand-new account, do not need a verified email. Set `OIDC_REQUIRE_EMAIL_VERIFIED=false` to restore link-by-unverified-email, which is only safe when the IdP guarantees the address.
- The display name is refreshed from `OIDC_NAME_CLAIM` on each login. The email is refreshed too, unless another user already has it.
- SSO-only users cannot use password login (`invalid credentials`) and cannot change a password (`this account has no local password`). A disabled user is turned away at the callback.
- New SSO users get no space memberships. They see `internal` spaces as viewers and need to be added to private ones.

### Admin group mapping

When `OIDC_ADMIN_GROUP` is set, group membership is authoritative on **every** SSO login: a user whose `OIDC_GROUPS_CLAIM` contains the group becomes a system admin, and a user who does not is demoted. The mapping never demotes the **last active system admin**: if the demotion would leave none, the flag is kept and the API logs `oidc admin-group mapping would demote the last active system admin ...`. The check runs under the same Postgres advisory lock as the admin console's demote and disable actions, so concurrent demotions cannot leave zero admins. Any other admin who is linked to an SSO identity and missing from the group loses admin at their next SSO sign-in.

When `OIDC_ADMIN_GROUP` is **unset**, SSO logins never change `is_system_admin`: they do not promote, and they do **not demote**. The bootstrap admin and anyone promoted in the admin console keep their role even if they sign in through SSO. (Backlog demotes everyone in this case; Knowledge does not.)

---

## Cookies and transport security

- The session cookie is `sid`: `HttpOnly`, `SameSite=Lax`, `Path=/`, valid for 30 days, holding a random token whose SHA-256 is the session row id. Expired sessions are deleted hourly. Disabling a user invalidates their sessions at the next request.
- Set `COOKIE_SECURE=true` whenever the site is served over HTTPS; the browser then refuses to send `sid` over plain HTTP. Over plain HTTP leave it `false`, or the browser drops the cookie and login appears to succeed but the session is lost.
- The OIDC flow sets short-lived (10 minute) `oidc_state`, `oidc_nonce`, and `oidc_next` cookies with the same flags. `next` is only honoured as a plain relative path: it must start with a single `/`, be at most 2048 bytes, and contain no control characters, backslashes, scheme, host, or userinfo.
- Same origin is the simplest setup: when the page and the API share a hostname (the cluster layout), no CORS configuration is involved. With separate origins, list the page origin in `APP_ORIGINS`; the API then allows credentialed requests from exactly those origins, with the `Content-Type` header and methods `GET, POST, PUT, PATCH, DELETE, OPTIONS`.
- Changing a password (`POST /auth/change-password`) deletes every other session of that user and keeps the current one. An admin password reset deletes all of the user's sessions. The web app sends a user whose session disappeared back to `/login`.
- The web app sends `Content-Security-Policy` (`default-src 'self'`, with `connect-src` and `img-src` extended by `NEXT_PUBLIC_API_BASE_URL`), `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, a strict `Referrer-Policy`, and a restrictive `Permissions-Policy`. `unsafe-eval` is added only by `next dev`.

### Rate limiting and proxies

Login and change-password are throttled twice, in memory per API process (see [API.md](API.md#rate-limits)): per client IP (burst 10, 10 per minute) and per account email (burst 5, 5 per minute).

- **The per-email limit locks out the real owner as well.** Every attempt costs a token, right or wrong, and the check runs before the password is hashed. Five bad attempts in a minute mean even the correct password answers `429 too many requests` until a token refills (one every 12 seconds). This is the accepted trade-off against online guessing; it also bounds argon2 work, since at most four hashes run concurrently.
- **`TRUSTED_PROXIES` decides what "client IP" means.** With it empty, `X-Forwarded-For` and `X-Real-IP` are ignored and the peer address is used, so a client cannot dodge the per-IP limit by sending a header. That is correct when browsers reach the API directly (local Compose), but behind a reverse proxy or ingress every user then shares the proxy's address and its single bucket. Set `TRUSTED_PROXIES` to the proxy's address range (on pi-lab, the pods that run the Tailscale ingress proxy) so the API reads the client from `X-Forwarded-For`: when the peer is in the list, the client is the right-most `X-Forwarded-For` entry that is not itself in the list, or `X-Real-IP` if there is no `X-Forwarded-For`. A malformed or oversized (over 4096 bytes) header leaves the peer address in place.
- **Only list addresses you control.** Anyone able to connect from a trusted range can claim any client IP. Do not trust a broad range such as a whole LAN.

### Server timeouts and database limits

The API sets `ReadHeaderTimeout` 10 s, `ReadTimeout` 60 s, `IdleTimeout` 120 s, `MaxHeaderBytes` 64 KiB, and no write timeout (downloads and the SSE stream stay open). `POST .../attachments` and `POST .../import` get a 5 minute read deadline and a 5 minute context timeout instead of 60 s, so slow large uploads can finish. Every pooled Postgres connection runs with `statement_timeout = 30s`. A reverse proxy in front must allow at least these durations and must not buffer SSE.

---

## Roles, permissions, and visibility

Space roles, from least to most privileged: **viewer, editor, admin**. **System admins** (`is_system_admin` on the user row) are implicit admins in every space. The matrix lives in `api/internal/domain/permissions.go` (`RoleAllows`).

| Permission | Viewer | Editor | Space admin | System admin |
|---|:-:|:-:|:-:|:-:|
| `view_space` | yes | yes | yes | yes |
| `comment` | yes | yes | yes | yes |
| `edit_pages` | no | yes | yes | yes |
| `manage_attachments` | no | yes | yes | yes |
| `manage_labels` | no | yes | yes | yes |
| `manage_templates` | no | yes | yes | yes |
| `manage_members` | no | no | yes | yes |
| `manage_space` | no | no | yes | yes |
| `purge_trash` | no | no | yes | yes |
| `delete_space` | no | no | yes | yes |

In terms of actions:

| Action | Viewer | Editor | Space admin | System admin |
|---|:-:|:-:|:-:|:-:|
| Read pages, history, attachments, labels, activity; search; export | yes | yes | yes | yes |
| Comment, reply, resolve threads, star, watch | yes | yes | yes | yes |
| Edit or delete own comments | edit and delete | edit and delete | edit and delete | edit and delete |
| Delete others' comments | no | no | yes | yes |
| Create, edit, publish, move, copy, trash, restore pages; restore a version; import | no | yes | yes | yes |
| View the space trash | no | yes | yes | yes |
| Upload and delete attachments; edit labels | no | yes | yes | yes |
| Create, edit, delete the space's templates | no | yes | yes | yes |
| Space settings (name, icon, visibility, home page), members, archive | no | no | yes | yes |
| Permanently delete trashed pages; delete the space | no | no | yes | yes |
| Create a space (becomes its admin) | yes | yes | yes | yes |
| Manage users; create or edit global templates; list every space | no | no | no | yes |

Comment editing is author-only for everyone, system admins included. A space must keep at least one `admin` member (`409 last_admin`), and the system must keep at least one active system admin.

### Visibility semantics

- **`internal`** (the default): every signed-in user is an implicit **viewer**. They can read, comment, star, watch, search, and export. A membership can raise their role but never lowers it below viewer.
- **`private`**: members and system admins only. Everyone else gets `404` for the space and everything in it, so its existence is not revealed. Search, label autocomplete, the dashboard, notifications, and the directory all exclude it for them.
- **Archived**: readable by everyone who could read it; writable only by space and system admins. Editors see a read-only banner and the API answers `403 space is read-only` to their writes.
- **Soft-deleted**: `404` for everyone except system admins. The key stays reserved.
- **Draft pages** (created but never published): visible only to their creator and space admins, in the tree, search, dashboard, comments, labels, attachments, and notifications.
- **Trashed pages**: visible to editors and admins in the trash view and by direct link; hidden from viewers on the page, its versions, comments, labels, attachments, and activity. A never-published page that was trashed stays visible only to its creator and space admins. The space home page can be neither trashed nor moved, directly or by trashing an ancestor.
- **Page tree depth** is capped at 64 levels (create, copy, move, restore, and import all refuse to go deeper).
- **Activity feed**: a row is shown only while its page still exists and is visible to the viewer by the rules above; rows about purged pages are never shown.

---

## First-boot admin bootstrap

On startup, the API:

1. Waits up to 30 seconds for Postgres, then runs `goose` migrations from `MIGRATIONS_DIR`.
2. Checks the `users` table.
3. If it is empty and `ADMIN_EMAIL` and `ADMIN_PASSWORD` are set, creates a user from `ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_DISPLAY_NAME` with `is_system_admin=true` and logs `admin bootstrapped`.
4. If it is not empty, logs `admin bootstrap skipped: users already exist` and ignores those variables. If the variables are unset it logs `admin bootstrap skipped: ADMIN_EMAIL / ADMIN_PASSWORD not set`.
5. A bootstrap password that fails the password policy aborts startup with `bootstrap: password must be at least 12 characters and include upper, lower, digit, and symbol`.
6. Creates the four built-in global templates (meeting notes, decision record, how-to guide, retrospective), authored by the first system admin, if the `templates` table is empty and an active system admin exists.

Manage accounts from **Admin** then **Users** after the first user exists.

## Recovering a lost admin

Changing `ADMIN_*` after the first boot does nothing, and clearing the `users` table is not an option once anyone has created content: spaces, pages, and comments reference their authors, so `DELETE FROM users` fails on a populated database. Use the narrowest option that applies. The commands assume the Compose stack; against another Postgres, run the same SQL with `psql`.

**1. Another system admin can sign in.** Open **Admin** then **Users** and reset the password, or grant system admin to someone else.

**2. Someone who can sign in is not an admin.** Promote them in SQL, then use the admin console:

```bash
docker compose exec db psql -U knowledge -d knowledge \
  -c "UPDATE users SET is_system_admin = true, disabled_at = NULL WHERE email = 'alice@example.com';"
```

**3. SSO is configured.** Set `OIDC_ADMIN_GROUP`, add yourself to that group at the IdP, restart the API, and sign in through SSO.

**4. Nobody can sign in.** Generate an argon2id hash with the parameters the API uses and write it into the admin's row. First produce the hash in a scratch directory outside the repo (needs Go):

```bash
mkdir /tmp/hashgen && cd /tmp/hashgen
cat > main.go <<'EOF'
package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"
)

func main() {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}
	key := argon2.IDKey([]byte(os.Args[1]), salt, 3, 32*1024, 2, 32)
	fmt.Printf("$argon2id$v=%d$m=32768,t=3,p=2$%s$%s\n", argon2.Version,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}
EOF
go mod init hashgen
go get golang.org/x/crypto@v0.58.0
go run . 'A-New-Passw0rd-123!'
```

Then, from the repo directory, paste the printed hash into the row:

```bash
HASH='$argon2id$v=19$m=32768,t=3,p=2$...'
docker compose exec -T db psql -U knowledge -d knowledge \
  -c "UPDATE users SET password_hash = '$HASH', is_system_admin = true, disabled_at = NULL WHERE email = 'admin@example.com';"
```

Sign in with the new password and change it. This path bypasses the password policy, so pick a compliant password.

**5. Nothing worth keeping.** `docker compose down -v` deletes the database and the stored attachments; the next `docker compose up` bootstraps from `ADMIN_*` again.

---

## Verification walkthrough

A manual smoke test to run against a fresh install and confirm everything is wired up. It needs two browser sessions (use a private window for the second).

1. `curl -s http://localhost:8080/health` returns `{"ok":true,"version":"dev"}`, and `curl -s http://localhost:8080/api/v1/auth/oidc/config` returns `{"enabled":false}` when OIDC is unset.
2. Open <http://localhost:3000> and sign in as the bootstrap admin.
3. **Admin** then **Users** then **New user**: create `alice@example.com` with a policy-compliant password.
4. **Spaces** then **Create space**: key `ENG`, name `Engineering`, visibility **private**.
5. In `ENG`, choose **Create page**, title it `Runbook`, and in the editor type `/` to open the slash menu. Add an info panel, a table, and a code block. Wait for the status to show **Saved**, then **Publish** with the message `first draft`.
6. The page opens at `/spaces/ENG/pages/<id>/runbook`. **More actions** then **History** lists one version.
7. **Edit**, change a sentence, and publish again. In History, compare the two versions and **Restore** the first: a third version appears with the message `Restored version 1`.
8. As Alice (second session), open `/spaces/ENG`: it is not found, because the space is private and she is not a member.
9. As the admin, open the space **Settings** then **Members** and add Alice as an **editor**. Reload as Alice: `ENG` now appears in her sidebar.
10. As Alice, watch the `Runbook` page (the eye icon beside **Edit**). As the admin, edit and publish it. Within a second or two Alice's bell shows an unread notification without a reload (SSE).
11. As Alice, select text in the page and add an **inline comment**; the text is highlighted. As the admin, reply and resolve the thread.
12. Add the label `ops` to the page. Use the top bar search for a word from the page: the hit shows highlighted snippets, and filtering by `ops` and by space `ENG` keeps it.
13. **More actions** then **Attachments**: upload a small file and download it. Uploading a file over `ATTACHMENT_MAX_BYTES` is refused with `413`. Drop an image into the editor and confirm it renders after publishing.
14. **More actions** then **Export** then **Markdown**, then **Settings** then **Import and export**: export the space as a zip and import a `.md` file; the imported page appears under the home page.
15. **More actions** then **Move to trash**. Open the space **Trash**, **Restore** the page, and confirm Alice (an editor) sees it again.
16. Change the space visibility to **internal** and, as a user who is not a member, confirm the space is readable but has no edit controls. **Archive** it and confirm Alice can no longer edit; **Unarchive** it.
17. In **Members**, demote the only admin: the request is refused with `last_admin` and a toast says a space must keep an admin.
18. As Alice, change her password in **Account** while signed in on two browsers: the other browser is signed out at its next request and returns to `/login`.
19. Restart the API (`docker compose restart api`): both sessions stay signed in (sessions are stored in Postgres), and the log shows `admin bootstrap skipped: users already exist`.
