# The dev Deployment

Pomerium in front, Keycloak as the Identity Provider, the Server behind them, everything on loopback. It runs the
Server the way [docs/server.md](../docs/server.md) says a Deployment does, with the same flags and the same admission
policy, so Reader behaviour and both login flows can be exercised by hand. Nothing here is tested in Go.

## Run

```sh
cd dev
./certs/make-certs.sh     # once: throwaway CA + wildcard cert for *.localhost.pomerium.io (public DNS -> 127.0.0.1)
./seed-origin.sh          # once: data/origin.git, a bare origin whose main is this checkout's HEAD
docker compose up -d --build --wait
```

Stop with `docker compose down`. Keycloak keeps its data inside the container, so `down` plus `up` re-imports the
realm from `keycloak/ubiq-realm.json`; edit that file, not the admin console, for anything that should last. The
Server's clone lives in the `ubiq-data` volume and its origin in `data/` (untracked): Batches the Server pushes land
in `data/origin.git`, where `git -C data/origin.git log --all` shows them. To start over, `docker compose down -v`,
`rm -rf data`, and seed again. After changing ubiq's code, `docker compose up -d --build ubiq`.

`docker compose logs -f ubiq` shows the Server's one line per request.

## The Server in the stack

The `ubiq` service is built from this checkout (`ubiq/Dockerfile`) and runs:

```sh
ubiq serve -root /data/repo -origin /origin.git -listen 0.0.0.0:8766 \
  -assertion-keys https://authenticate.localhost.pomerium.io/.well-known/pomerium/jwks.json \
  -ca /certs/ca.pem -writer-claim groups -writers ubiq-writers \
  -committer "ubiq serve <ubiq@localhost>"
```

It differs from production only where the stack forces it. It listens on all of its container's interfaces, since
Pomerium reaches it across the compose network, and publishes no port, so the only way in is through the proxy. Its
origin is a local bare repository, not HTTPS. It trusts the throwaway CA to fetch Pomerium's assertion keys, reaching
the authenticate host through a compose network alias. The container clones the origin into `-root` before the first
start.

**Dev direct mode** needs none of this: the Server alone on the host, every request one fixed Writer.

```sh
./dev/seed-origin.sh
git clone dev/data/origin.git /tmp/ubiq-direct    # the Server reuses a clone it finds
go run . serve -root /tmp/ubiq-direct -origin dev/data/origin.git -writers dev -person "Ada Lovelace <ada@example.com>"
```

## Where things are

| What | Where |
|---|---|
| Keycloak admin console | http://localhost:8080/admin, `admin` / `admin` |
| Keycloak realm `ubiq`, issuer | `http://keycloak.localhost.pomerium.io:8080/realms/ubiq` (browser and Pomerium both resolve this name) |
| Pomerium authenticate URL | `https://authenticate.localhost.pomerium.io` |
| Pomerium assertion keys | `https://authenticate.localhost.pomerium.io/.well-known/pomerium/jwks.json` |
| The page | `https://ubiq.localhost.pomerium.io` |
| The MCP route | `https://mcp.localhost.pomerium.io` (the MCP endpoint is `/mcp`) |
| Pomerium config | `pomerium/config.yaml` |
| Client credentials | client `pomerium`, secret in `pomerium/config.yaml` and `keycloak/ubiq-realm.json` |
| CA to trust | `certs/ca.pem` (untracked; `NODE_EXTRA_CA_CERTS=dev/certs/ca.pem` for Node clients such as Claude Code) |

## Test people

| Username | Password | Email | `groups` claim | Role |
|---|---|---|---|---|
| `writer` | `writer` | writer@ubiq.test | `["ubiq-writers"]` | Writer |
| `reader` | `reader` | reader@ubiq.test | absent | Reader |

Keycloak puts `groups` in the ID token through a group-membership mapper on the `pomerium` client. Pomerium Core
copies it into its assertion JWT (`X-Pomerium-Jwt-Assertion`) unchanged, so the Writer Rule, claim `groups` with
value `ubiq-writers`, decides the role. Both routes admit any `authenticated_user`; Pomerium refuses no one Keycloak
logged in. Both people hold the `offline_access` realm role because Pomerium asks for that scope by default and
Keycloak refuses the code exchange without it.

## Exercising it by hand

```sh
./login.sh writer writer      # {"name":"Wren Writer","role":"Writer"}
./login.sh reader reader      # {"name":"Rae Reader","role":"Reader"}
./mcp-login.sh writer writer /tmp/writer.tok
./mcp-login.sh reader reader /tmp/reader.tok
./mcp-call.sh /tmp/reader.tok define_term '{"name":"Batch Window","definition":"The time a Batch stays open."}'
./mcp-call.sh /tmp/writer.tok define_term '{"name":"Batch Window","definition":"The time a Batch stays open."}'
```

- `login.sh <user> <password>` plays a browser with curl: opens the page, follows Pomerium to Keycloak's form, logs
  in, and prints the Person and role the page shows.
- `mcp-login.sh <user> <password> [token-file]` plays an MCP client as Claude Code connects: client ID
  `https://claude.ai/oauth/claude-code-client-metadata`, PKCE authorization code, Keycloak's form, token, then
  `whoami`, which answers the Keycloak Person, and `lookup` over the MCP route. The token lasts 14 hours.
- `mcp-call.sh <token-file> <tool> [json]` calls any tool with a saved token and prints its text, prefixed
  `REFUSED:` when the tool refused. The Reader's `define_term` is refused in one sentence naming the Writer Rule's
  claim and values; the Writer's lands as a commit on `ubiq/<n>` in `data/origin.git`.

To connect Claude Code itself: `NODE_EXTRA_CA_CERTS=dev/certs/ca.pem claude mcp add --transport http ubiq-dev
https://mcp.localhost.pomerium.io/mcp`, then log in from `/mcp`.

## Pomerium facts observed on v0.34.0

These shaped the Server and the configuration; a successor proxy has to answer each.

- **Host forwarding.** Pomerium forwards requests with the upstream's `Host` (`ubiq:8766` here). The MCP Go SDK's
  DNS-rebinding guard rejects a non-localhost `Host` on a loopback listener with `403 invalid Host header`, so the
  Server sets `DisableLocalhostProtection`.
- **The OAuth metadata probe.** On every authorize Pomerium probes the upstream for its own OAuth metadata (`GET
  /.well-known/oauth-protected-resource` at the Server), logs a warning because the upstream is plain `http`, and
  proceeds. Harmless noise; the Server logs one line for it.
- **Identity headers.** Only with `pass_identity_headers: true` does the assertion JWT reach the Server on MCP
  requests, so `whoami` over MCP and the page see the same Person. Without it the Server sees no Person on MCP
  calls and answers `401`.
- **Client host allow-listing.** Clients register one of two ways, and both are off until configured. A client ID
  metadata document (Claude Code's is `https://claude.ai/oauth/claude-code-client-metadata`) needs its host in
  `mcp_allowed_client_id_domains`. Dynamic client registration needs the runtime flag
  `mcp_dynamic_client_registration`; without it the metadata has no `registration_endpoint` and
  `/.pomerium/mcp/register` answers `403`.
- An unauthenticated `POST /mcp` answers `401` with `WWW-Authenticate: Bearer resource_metadata=".../.well-known/oauth-protected-resource/mcp"`.
- The MCP route takes bearer tokens only: a browser session cookie does not open it.
- Loopback redirect URIs match regardless of port (OAuth 2.1), so Claude Code's portless `http://localhost/callback`
  works.
- Authorize and token endpoints live under `/.pomerium/mcp/` on the MCP route's own host. Access tokens last 14 hours
  (`expires_in: 50399`); a refresh token comes with them.
