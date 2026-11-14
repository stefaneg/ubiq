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
| MCP route, for the Server on the host at port 8766 | `https://mcp.localhost.pomerium.io` |
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
received: no browser needed.

## MCP facts observed on Pomerium v0.34.0

- An unauthenticated `POST /mcp` answers `401` with `WWW-Authenticate: Bearer resource_metadata=".../.well-known/oauth-protected-resource/mcp"`.
- The authorization-server metadata advertises `client_id_metadata_document_supported: true`, PKCE `S256`, and **no `registration_endpoint`**: no dynamic client registration.
- Authorize and token endpoints live under `/.pomerium/mcp/` on the MCP route's own host.
