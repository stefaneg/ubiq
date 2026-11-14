# The dev Deployment

Pomerium in front, Keycloak as the Identity Provider, everything on loopback. The Server joins later; until then the
`verify` container echoes the identity Pomerium forwards.

## Run

```sh
cd dev
./certs/make-certs.sh     # once: throwaway CA + wildcard cert for *.localhost.pomerium.io (public DNS -> 127.0.0.1)
docker compose up -d --wait
```

Stop with `docker compose down`. Keycloak keeps its data inside the container, so `down` plus `up` re-imports the
realm from `keycloak/ubiq-realm.json`; edit that file, not the admin console, for anything that should last.

## Where things are

| What | Where |
|---|---|
| Keycloak admin console | http://localhost:8080/admin, `admin` / `admin` |
| Keycloak realm `ubiq`, issuer | `http://keycloak.localhost.pomerium.io:8080/realms/ubiq` (browser and Pomerium both resolve this name) |
| Pomerium authenticate URL | `https://authenticate.localhost.pomerium.io` |
| Pomerium assertion keys | `https://authenticate.localhost.pomerium.io/.well-known/pomerium/jwks.json` |
| Identity echo | `https://verify.localhost.pomerium.io` (`/json` for the raw claims) |
| Page route, for the Server on the host at port 8766 | `https://ubiq.localhost.pomerium.io` |
| MCP route, same Server | `https://mcp.localhost.pomerium.io` (the MCP endpoint is `/mcp`) |
| Pomerium config | `pomerium/config.yaml` |
| Client credentials | client `pomerium`, secret in `pomerium/config.yaml` and `keycloak/ubiq-realm.json` |
| CA to trust | `certs/ca.pem` (untracked; `NODE_EXTRA_CA_CERTS=dev/certs/ca.pem` for Node clients such as Claude Code) |

## Test people

| Username | Password | Email | `groups` claim |
|---|---|---|---|
| `writer` | `writer` | writer@ubiq.test | `["ubiq-writers"]` |
| `reader` | `reader` | reader@ubiq.test | absent |

Keycloak puts `groups` in the ID token through a group-membership mapper on the `pomerium` client. Pomerium Core
copies it into its assertion JWT (`X-Pomerium-Jwt-Assertion`) unchanged, so the Writer Rule's default of claim
`groups` with value `ubiq-writers` can be exercised. Both people hold the `offline_access` realm role because Pomerium
asks for that scope by default and Keycloak refuses the code exchange without it.

`./login.sh writer writer` logs a person in through Keycloak with curl and prints the claims the verify route
received: no browser needed. `./mcp-login.sh writer writer` does the same as an MCP client would: dynamic
registration, PKCE authorization code, Keycloak login, token, then `whoami` and `lookup` over the MCP route.

## The prototype Server

`go run . serve` (see `proto_serve.go`, throwaway) listens on `127.0.0.1:8766` and serves the glossary page at `/`
with a signed-in banner, `/whoami`, and the MCP tools at `/mcp` plus a `whoami` tool. It reads the Person from
`X-Pomerium-Jwt-Assertion`, verified against Pomerium's JWKS with the Pomerium Go SDK; `groups` containing
`ubiq-writers` makes a Writer, anything else a Reader.

## MCP facts observed on Pomerium v0.34.0

- An unauthenticated `POST /mcp` answers `401` with `WWW-Authenticate: Bearer resource_metadata=".../.well-known/oauth-protected-resource/mcp"`.
- The MCP route takes bearer tokens only: a browser session cookie does not open it.
- Clients register one of two ways, and both are off until configured. A client ID metadata document (Claude Code's
  is `https://claude.ai/oauth/claude-code-client-metadata`) needs its host in `mcp_allowed_client_id_domains`.
  Dynamic client registration needs the runtime flag `mcp_dynamic_client_registration`; without it the metadata
  has no `registration_endpoint` and `/.pomerium/mcp/register` answers `403`.
- Loopback redirect URIs match regardless of port (OAuth 2.1), so Claude Code's portless `http://localhost/callback` works.
- Authorize and token endpoints live under `/.pomerium/mcp/` on the MCP route's own host. Access tokens last 14 hours
  (`expires_in: 50399`); a refresh token comes with them.
- On every authorize Pomerium probes the upstream for its own OAuth (`GET /.well-known/oauth-protected-resource` at
  the Server), logs a warning because the upstream is plain `http`, and proceeds. Harmless noise.
- Pomerium forwards requests with the upstream's `Host` (`host.docker.internal:8766`). The MCP Go SDK's
  DNS-rebinding guard rejects a non-localhost `Host` on a loopback listener with `403 invalid Host header`, so the
  Server sets `DisableLocalhostProtection`.
- With `pass_identity_headers: true` the assertion JWT reaches the Server on MCP requests too, so `lookup` over MCP
  and the page see the same Person.
