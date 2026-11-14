# How MCP clients authenticate to a remote server

Research for [#5](https://github.com/stefaneg/ubiq/issues/5), part of map [#4](https://github.com/stefaneg/ubiq/issues/4).
Checked 2026-11-14 against primary sources only: the MCP specification, the
`modelcontextprotocol/go-sdk` source at commit `0fa5e84` (2026-11-13), the
official documentation of each client, and the official documentation (or, where
the documentation is silent, the live discovery document or source code) of each
identity provider. Every claim carries its source. Where a primary source did not
settle a point, the text says so.

## Short answer

An ordinary OpenID Connect provider cannot, in general, serve as the
authorization server that MCP clients talk to. Served Ubiq should be its own
OAuth 2.1 authorization server, log the person in upstream through the
organisation's OpenID Connect provider, and issue its own access tokens.

Three independent facts force this:

1. **Clients need a client id without anyone registering one.** The current MCP
   specification (2026-07-28) gives three ways to get it: Client ID Metadata
   Documents (CIMD, the recommended one), pre-registration, and Dynamic Client
   Registration (DCR, deprecated but still what most clients fall back to). None
   of the five providers named in the ticket supports CIMD. Google and Microsoft
   Entra publish no registration endpoint at all. Okta and Authentik publish one
   but require an already-authorised bearer token to call it, which an MCP client
   does not have. Keycloak alone can do open DCR, and only after an administrator
   whitelists trusted hosts. That leaves pre-registration: every person using
   Claude Code, Cursor, VS Code or the Inspector would type a client id (and the
   provider would have to allow each client's loopback redirect URI), and a
   claude.ai administrator would enter one for the organisation.

2. **The server must verify the token's audience, and most providers' tokens do
   not carry a usable one.** The spec makes the MCP server reject any token not
   issued for it. Google access tokens are opaque and their audience is the OAuth
   client, not the resource. Authentik sets `aud` to the client id. Okta needs a
   licensed custom authorization server to mint audience-bound JWTs. Entra and
   Keycloak can do it, Entra only with an app registration per MCP server and v2
   tokens, Keycloak only with client scopes or audience mappers configured per
   realm. Each is provider-specific setup, which the map rules out.

3. **Clients must refuse providers whose discovery document lacks
   `code_challenge_methods_supported`.** Entra's does, as of today. A client that
   follows the spec cannot start the flow against it.

The alternative pattern is well established: the MCP server (or a sidecar) is the
authorization server, delegates login to any OpenID Connect provider through
standard discovery, and issues tokens for itself. Cloudflare's `workers-oauth-provider`
is the reference example and ships upstream-login helpers for exactly this. In
Go there is no drop-in library: `ory/fosite` and `zitadel/oidc` give the
protocol plumbing, but the authorize endpoint, consent page, upstream OIDC
round-trip, storage and token format are the implementer's. The go-sdk gives the
resource-server half (`auth.RequireBearerToken`, protected resource metadata)
and the client half, not an authorization server. What has to be built is listed
in section 4.

What this buys: any provider through discovery, no provider-specific code, every
named client works unaided (CIMD for Claude, VS Code and the Inspector; DCR for
Cursor and Claude Code; pre-registration still available), and the roles and
commit identity come from the ID token claims Ubiq already needs.

## 1. The current MCP authorization specification

**Version.** The current protocol revision is `2026-07-28`. Earlier revisions
(`2025-11-25`, `2025-06-18`) are final. Source:
<https://modelcontextprotocol.io/specification/versioning>. The go-sdk supports
all three (`mcp/shared.go` constants `protocolVersion20260728`, `...20251125`,
`...20250618`).

**Roles.** "A protected MCP server acts as an OAuth 2.1 resource server"; "An
MCP client acts as an OAuth 2.1 client"; the authorization server "may be
hosted with the resource server or a separate entity" and its implementation
"is beyond the scope of this specification". Source:
<https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization>.

**What the server MUST do** (same source unless noted):

- Implement OAuth 2.0 Protected Resource Metadata (RFC 9728) with an
  `authorization_servers` field, and advertise it either in the
  `WWW-Authenticate` header of a 401 (`resource_metadata="..."`) or at
  `/.well-known/oauth-protected-resource[/path]`. Source:
  <https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/authorization-server-discovery>.
- Validate access tokens per OAuth 2.1 §5.2 and "validate that access tokens
  were issued specifically for them as the intended audience, according to RFC
  8707 Section 2". "MCP servers MUST NOT accept or transit any other tokens."
- Return 401 for invalid or expired tokens, 403 with
  `error="insufficient_scope"` and a `scope` parameter for step-up.
- SHOULD include a `scope` parameter in the 401 challenge.
- If it calls upstream APIs it "MUST NOT pass through the token it received
  from the MCP client". Source:
  <https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/security-considerations>.

**What the authorization server MUST do:**

- "Authorization servers MUST implement OAuth 2.1".
- Provide at least one of RFC 8414 metadata
  (`/.well-known/oauth-authorization-server`) or OpenID Connect Discovery
  (`/.well-known/openid-configuration`). Clients MUST try both, in that order,
  with path-insertion variants for issuers with path components.
- Include `code_challenge_methods_supported` in its metadata; "If the field is
  absent, MCP clients MUST refuse to proceed." Source: security-considerations
  page above, "Authorization Code Protection".
- SHOULD support Client ID Metadata Documents and advertise it with
  `client_id_metadata_document_supported: true`; MAY support RFC 7591 DCR, which
  "is deprecated and retained for backwards compatibility". Source:
  <https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/client-registration>.
- Validate exact redirect URIs; SHOULD emit RFC 9207 `iss` in authorization
  responses (a future revision is expected to make this MUST).
- If it is a proxy using a static client id at a third-party authorization
  server, it "MUST obtain user consent for each dynamically registered client
  before forwarding to third-party authorization servers" (confused deputy).

**What the client MUST do:**

- Use PKCE with `S256` and refuse to proceed without
  `code_challenge_methods_supported`.
- Send the RFC 8707 `resource` parameter (the MCP server's canonical URI) in
  both the authorization and token requests, "regardless of whether
  authorization servers support it".
- Obtain a client id by one of three mechanisms, in this priority: (1)
  pre-registered client information if it has it; (2) CIMD if the AS advertises
  `client_id_metadata_document_supported`; (3) DCR if the AS has a
  `registration_endpoint`; (4) "Prompt the user to enter the client
  information if no other option is available". Clients SHOULD support
  pre-registration; for DCR they MUST send `application_type` (`native` for
  CLI/desktop/localhost clients).
- Record the AS `issuer` before redirecting and validate RFC 9207 `iss`.
- Bind stored client credentials to the AS issuer and re-register if the AS
  changes.

**What is left to the client / unspecified:** scope selection when the
challenge carries none (fall back to `scopes_supported`), whether to persist
refresh tokens, token storage, the user interface for pre-registered
credentials, and which authorization server to pick when several are listed.

## 2. What the clients support

### Claude Code

Source: <https://code.claude.com/docs/en/mcp>.

- OAuth 2.0 on HTTP transport; a 401 or 403 marks the server as needing
  authentication; `/mcp` or `claude mcp login <name>` runs the browser flow.
  Tokens "are stored securely and refreshed automatically".
- **DCR is the default**: `claude mcp add --transport http ...` with no client
  flags. Error text when the AS lacks it: "Incompatible auth server: does not
  support dynamic client registration".
- **CIMD**: "Claude Code also supports servers that use a Client ID Metadata
  Document (CIMD) instead of Dynamic Client Registration, and discovers these
  automatically."
- **Pre-registered client**: `--client-id <id> [--client-secret]
  [--callback-port N]`, or an `oauth: {clientId, callbackPort, scopes,
  authServerMetadataUrl}` object in JSON config; secret via masked prompt or
  `MCP_CLIENT_SECRET` at add time only. Redirect URI to register at the
  provider: `http://localhost:PORT/callback`.
- **Static bearer**: `--header "Authorization: Bearer ..."` or `headersHelper`
  disables OAuth for that server.
- Discovery order: RFC 9728 at `/.well-known/oauth-protected-resource`, then
  RFC 8414 at `/.well-known/oauth-authorization-server`.
- Non-interactive runs (`claude -p`, Agent SDK) cannot run the OAuth flow; sign
  in from an interactive session first.

### Claude Desktop and claude.ai (custom connectors)

Source: <https://claude.com/docs/connectors/custom/add-unlisted>. This page
covers "Claude" across plans; the Desktop app signs in to claude.ai and uses
the same connectors. I found no separate Desktop-only OAuth documentation.

- Three **OAuth client** options when adding a connector: "Use Claude's
  published identity" (CIMD, "the recommended option ... the server must
  support this method"); "Register automatically" (DCR, "works with most
  servers and adds client registrations over time"); "Use your own OAuth
  client" (pre-registered id, secret optional). The dialog pre-selects what it
  detects from the server's metadata.
- Fixed credentials go in **Request headers** (beta, limited organisations);
  `Authorization` cannot be a request header on an OAuth connection.
- On Team and Enterprise plans an Owner adds the connector; members then
  connect with their own account. Authentication settings cannot be changed
  after creation.
- Callback URL: a search summary of the claude.com authentication docs reports
  `https://claude.ai/api/mcp/auth_callback` with client name "Claude" and
  advises also allow-listing a future claude.com callback. I did not see this
  in the page I fetched; verify at
  <https://claude.com/docs/connectors/building/authentication> before
  registering a client.

### Cursor

Source: <https://cursor.com/docs/context/mcp>.

- Streamable HTTP and SSE; "Cursor supports OAuth and uses the authorization
  code flow with PKCE", reads RFC 9728 metadata and sends the RFC 8707 resource
  indicator.
- **DCR**: "With dynamic client registration, Cursor registers as a public
  client with no client secret."
- **Static OAuth** for providers that lack DCR or require redirect allow-listing:
  `auth: { CLIENT_ID, CLIENT_SECRET (optional), scopes (optional) }`, with
  `${env:...}` interpolation. Without `scopes` Cursor reads `scopes_supported`
  from `/.well-known/oauth-authorization-server`.
- Callback URLs to register: desktop `http://localhost:8787/callback`; web and
  Cursor Agents `https://www.cursor.com/agents/mcp/oauth/callback`. With DCR
  the desktop app registers both.
- Headers with bearer tokens are supported (`"Authorization": "Bearer
  ${env:MY_SERVICE_TOKEN}"`).
- **CIMD is not mentioned** in the Cursor documentation. Unverified either way.

### VS Code (GitHub Copilot)

Sources: <https://code.visualstudio.com/updates/v1_106>,
<https://code.visualstudio.com/updates/v1_123>,
<https://code.visualstudio.com/docs/agents/reference/mcp-configuration>.

- **CIMD** since 1.106: "When connecting to an MCP server that uses an
  authorization server that supports CIMD, VS Code will automatically use that
  flow over DCR."
- **DCR** is the older default (the 1.106 note contrasts CIMD with it).
- **Pre-registered client** since 1.123: `oauth: { clientId }` on an HTTP/SSE
  server entry; "When a client ID is set, VS Code uses it during the OAuth
  flow instead of registering a new client." A client secret is stored through
  the "Set Client Secret" CodeLens, never in `mcp.json`. The portable Copilot
  format uses a flat `oauthClientId`.
- Bearer headers: `headers: { "Authorization": "Bearer ${input:api-token}" }`.
- Fallback to prompting the user for a client id when DCR fails is described
  in the VS Code issue tracker, not in the docs I fetched. The redirect URI VS
  Code registers is not stated in the pages I read. Both unverified.

### MCP Inspector

Sources: <https://github.com/modelcontextprotocol/inspector> at commit
`488c30d` (2026-11-11), files `README.md`, `docs/environment-variables.md`,
`docs/test-servers.md`, `clients/cli/README.md`, `core/auth/cimd.ts`,
`clients/web/src/components/groups/ClientSettingsForm/clientSettingsValues.ts`.

- Full OAuth flow in the web, TUI and CLI clients; `--oauth-debug` traces
  protected-resource metadata, AS metadata, client registration, authorization
  and token requests. Default CLI/TUI loopback redirect:
  `http://127.0.0.1:6276/oauth/callback` (`MCP_OAUTH_CALLBACK_URL`).
- **DCR**: the documented trace shows `POST .../register` when the AS
  advertises `registration_endpoint`.
- **CIMD**: configured install-wide in `client.json` as `cimd: { enabled,
  clientMetadataUrl }`; `core/auth/cimd.ts` pre-registers `{ client_id:
  clientMetadataUrl }` "when the authorization server supports URL-based
  client IDs (SEP-991 / CIMD)". The user must host the metadata document.
- **Pre-registered client**: per-server `clientId` / `clientSecret` fields in
  Server Settings (`oauthClientSecret` is redacted by `servers/show`).
- Static bearer via per-server `headers`; `--stored-auth-only` for CI.
- Also supports Enterprise Managed Authorization (SEP-990) settings.

### Go SDK client (`github.com/modelcontextprotocol/go-sdk`)

Source: repository at commit `0fa5e84`, packages `auth`, `oauthex`,
`auth/extauth`, `mcp/streamable.go`.

- `mcp.StreamableClientTransport.OAuthHandler` takes an `auth.OAuthHandler`
  (`TokenSource`, `Authorize` on 401/403).
- `auth.AuthorizationCodeHandler` implements the whole flow: RFC 9728 lookup
  from the `WWW-Authenticate` challenge or the two well-known paths, AS
  metadata (RFC 8414 then OIDC discovery), PKCE support check
  (`authutil.CheckPKCESupport`), `resource` parameter on both the
  authorization and token requests, RFC 9207 `iss` validation, step-up on
  `insufficient_scope`, optional `offline_access`.
- **Client registration**: all three mechanisms via
  `ClientIDMetadataDocumentConfig`, `PreregisteredClient
  (*oauthex.ClientCredentials)`, `DynamicClientRegistrationConfig`. "At least
  one method must be configured." The handler tries CIMD first (when the AS
  advertises `client_id_metadata_document_supported`), then the pre-registered
  client, then DCR (when `registration_endpoint` is present). Note this order
  puts CIMD before pre-registration, the reverse of the spec's priority list.
- **Browser and redirect are the caller's**: `AuthorizationCodeFetcher` is a
  required callback that must open the URL and return the code; `RedirectURL`
  is required. The `examples/auth/client` runs a loopback listener on
  `localhost:<port>`.
- `auth/extauth` implements SEP-990 Enterprise Managed Authorization (OIDC
  login at the IdP, RFC 8693 token exchange to an ID-JAG, RFC 7523 JWT bearer
  grant at the MCP server's AS) and client-credentials.
- **Server side**: `auth.RequireBearerToken(verifier, opts)` middleware emits
  the 401/403 challenges with `resource_metadata` and scopes;
  `auth.ProtectedResourceMetadataHandler` serves RFC 9728;
  `oauthex.MatchesResource` compares `aud` with the resource. The
  `TokenVerifier` is supplied by the application: the SDK ships no JWT or
  JWKS verifier. `examples/auth/server` verifies by RFC 7662 introspection
  with client credentials and checks `aud`. There is no authorization server
  in the SDK; the ROADMAP lists client OAuth and the ext-auth extensions only.

## 3. How the providers fare as the authorization server

Three questions per provider: can an MCP client get a client id unaided (CIMD
or open DCR), does the access token carry an audience the MCP server can
verify, and is the token verifiable at all.

### Google

- **Client id**: credentials come from the Cloud console ("obtain OAuth 2.0
  client credentials from the Google API Console",
  <https://developers.google.com/identity/protocols/oauth2>). The live
  discovery document `https://accounts.google.com/.well-known/openid-configuration`
  has no `registration_endpoint` and no `client_id_metadata_document_supported`
  (checked 2026-11-14). It does include `code_challenge_methods_supported`, and
  `/.well-known/oauth-authorization-server` answers 200.
- **Token format**: "Some access tokens are opaque, meaning they are in a
  proprietary format and can't be inspected"; user access tokens are opaque and
  expire after one hour. Inspection only via
  `https://oauth2.googleapis.com/tokeninfo`, which returns `aud` as "The OAuth
  client that this token is for, identified by its OAuth client ID".
  Source: <https://docs.cloud.google.com/docs/authentication/token-types>.
- **Audience**: the client id, never a resource URI. Google documents no RFC
  8707 `resource` support (unverified: not found, not denied). An MCP server
  could only accept the token by calling `tokeninfo` per request and checking
  that `aud` equals a client id it trusts, which is not what the spec means by
  "issued specifically for them".

### Okta

- **Two kinds of authorization server.** The org authorization server's tokens
  "aren't intended for validation or use by your own apps or resource
  servers"; custom authorization servers "allow you to define and apply
  authorization policies to secure your own APIs", and need the API Access
  Management product ("an optional add-on in production environments"; the
  Integrator Free Plan includes a `default` one). Source:
  <https://developer.okta.com/docs/concepts/auth-servers/>.
- **Client id**: the custom AS metadata example lists `"registration_endpoint":
  "https://{yourOktaDomain}/oauth2/v1/clients"` and
  `"code_challenge_methods_supported": ["S256"]`, no
  `client_id_metadata_document_supported`
  (<https://developer.okta.com/docs/api/openapi/okta-oauth/oauth/customas/getwellknownoauthconfigurationcustomas>).
  `POST /oauth2/v1/clients` requires the OAuth 2.0 scope
  `okta.clients.register`
  (<https://developer.okta.com/docs/api/openapi/okta-oauth/oauth/client/createclient>);
  the API "largely follows ... RFC7591"
  (<https://developer.okta.com/docs/reference/api/oauth-clients/>). An
  unauthenticated MCP client cannot call it. Okta's own MCP-server features
  (Cross App Access, Brokered Consent) describe pre-registered or
  Okta-brokered clients; a search summary of
  <https://help.okta.com/oie/en-us/content/topics/ai-agents/ai-agent-mcp-server.htm>
  says Okta "only works with MCP servers that have preregistered OAuth
  clients", but the page I fetched did not contain that sentence. Unverified.
- **Token format and audience**: custom AS access tokens are RS256 JWTs
  validated locally against the JWKS; "verify that the `aud` (audience) claim
  equals the audience that is configured for your authorization server"
  (<https://developer.okta.com/docs/guides/validate-access-tokens/main/>). The
  audience is a setting of the custom AS, so a single AS serves one resource
  audience. RFC 8707 `resource` support: not found in the pages read.

### Microsoft Entra ID

- **Client id**: "Microsoft Entra ID doesn't publish an RFC 7591 registration
  endpoint, so there's nothing ... to register a client against. For a server
  that's protected by Microsoft Entra ID, register the OAuth client statically
  instead."
  (<https://learn.microsoft.com/en-us/microsoft-365/copilot/extensibility/plugin-authentication-dynamic-client-registration>).
  The live `https://login.microsoftonline.com/common/v2.0/.well-known/openid-configuration`
  has no `registration_endpoint`, no `client_id_metadata_document_supported`,
  and **no `code_challenge_methods_supported`** (checked 2026-11-14);
  `/.well-known/oauth-authorization-server` answers 404. Per the MCP spec a
  conforming client "MUST refuse to proceed" without that field.
- **Token format and audience**: Microsoft documents Entra as an MCP
  authorization server. The MCP server gets an app registration with
  `requestedAccessTokenVersion: 2` and an Application ID URI equal to its
  canonical URL; "Microsoft Entra ID compares that `resource` value against
  the Application ID URI ... If they match, Microsoft Entra ID issues a token
  whose audience (`aud`) claim is your MCP server." PRM must list
  `https://login.microsoftonline.com/<tenant-id>/v2.0`. Tokens are RS256 JWTs
  validated via the OIDC metadata and `jwks_uri`; APIs "must only accept tokens
  containing one of their AppId URIs as the `aud` claim". Sources:
  <https://learn.microsoft.com/en-us/entra/agent-id/secure-mcp-server-with-entra-id>,
  <https://learn.microsoft.com/en-us/entra/identity-platform/access-tokens>.
- Net: verifiable, audience-bound tokens, but only after Entra-specific
  registration of the MCP server, and every MCP client must be pre-registered
  (with every loopback redirect URI it uses).

### Keycloak

- **Client id**: "Keycloak implements OpenID Connect Dynamic Client
  Registration" at `/realms/<realm>/clients-registrations/openid-connect`,
  listed in the discovery document. Anonymous registration exists but "By
  default, there is not any whitelisted host, so anonymous client registration
  is de-facto disabled"; the recommended path is an administrator-issued
  initial access token. Policies: Trusted Hosts, Consent Required, Max Clients
  (default 200 for anonymous), Client Disabled. Source:
  <https://www.keycloak.org/securing-apps/client-registration>. CIMD: not
  found in the documentation; unverified.
- **Token format**: "Keycloak issued access tokens are JSON Web Tokens (JWT)
  digitally signed and encoded using JSON Web Signature (JWS)"; validate
  locally with the realm's JWK or via introspection (confidential clients
  only). Source: <https://www.keycloak.org/securing-apps/oidc-layers>.
- **Audience**: `aud` "should typically represent client ids of all services
  where the token is supposed to be used"; added either by the Audience
  Resolve mapper (client roles) or a hardcoded Audience mapper on a client
  scope the client requests (`scope=service1-scope` yields `"aud": "service1"`).
  Source: Server Administration Guide, "Audience support",
  <https://www.keycloak.org/docs/latest/server_admin/index.html>. The resource
  server must be modelled as a Keycloak client; RFC 8707 `resource` support is
  not mentioned in the pages read.
- Net: the only provider of the five that can do open DCR plus audience-bound
  JWTs, at the price of realm-specific configuration (trusted hosts or initial
  access tokens, a client per resource server, an audience mapper or client
  scope).

### Authentik

- **Client id**: DCR exists from authentik 2026.8.0, but "The client
  authenticates to the registration endpoint using a Bearer access token that
  includes the required DCR scope" (`goauthentik.io/oidc/dcr`); it is "not an
  anonymous or open registration unless the configured policies explicitly
  allow that behavior"; each registration creates a new application and
  provider that persists until deleted; no RFC 7592 management. Source:
  <https://docs.goauthentik.io/add-secure-apps/providers/oauth2/dynamic-client-registration>.
  An MCP client arriving with no token cannot register.
- **Token format**: "JWTs created by authentik are always signed" (RS256 with
  a signing key, otherwise HS256 with the client secret); JWKS at
  `/application/o/<slug>/jwks/`; introspection at `/application/o/introspect/`
  for confidential providers. Source:
  <https://docs.goauthentik.io/add-secure-apps/providers/oauth2/>.
- **Audience**: not documented on the provider page. In source,
  `IDToken.new` sets `id_token.aud = provider.client_id` and `to_access_token`
  reuses that dictionary and adds `azp = provider.client_id`
  (<https://raw.githubusercontent.com/goauthentik/authentik/main/authentik/providers/oauth2/id_token.py>).
  So `aud` is the client, not a resource. Token exchange (2026.8+) can mint a
  token whose `aud` is another provider's client id
  (<https://docs.goauthentik.io/add-secure-apps/providers/oauth2/token_exchange/>),
  which is an extra hop the MCP flow does not include.

## 4. The alternative: the MCP server as its own authorization server

**Pattern.** The MCP server (or a sibling service) is the OAuth 2.1
authorization server the clients discover through RFC 9728. Its `/authorize`
endpoint authenticates the person by redirecting to the organisation's OpenID
Connect provider (standard authorization code flow with PKCE, discovered from
the issuer URL), shows a consent page naming the MCP client, then issues its
own authorization code and access token bound to the MCP server as audience.
The ID token claims (`sub`, `name`, `email`, group or role claim) drive roles
and commit identity. Upstream tokens never reach the MCP client, and the
upstream provider sees one pre-registered confidential client: the MCP
server's.

**Who does this.** Cloudflare documents it as the primary way to protect a
remote MCP server: "The authorization server signs users in and issues access
tokens"; the `/authorize` page "signs the user in, asks for consent, and calls
`completeAuthorization()`"; `beginUpstream()` and `finishUpstream()` "send the
user to the provider after consent, and finish when it redirects back"; the
provider's token can be kept encrypted in `props`. Sources:
<https://developers.cloudflare.com/agents/model-context-protocol/protocol/authorization/>,
<https://developers.cloudflare.com/agents/model-context-protocol/guides/securing-mcp-server/>,
<https://github.com/cloudflare/workers-oauth-provider> (OAuth 2.1, RFC 7591,
"client registration (pre-registered, CIMD, DCR)", tokens stored as hashes).
The Cloudflare guide also states the spec's confused-deputy rule in practice:
"Ask for consent first, then hand the user to the provider", otherwise "an
attacker's client can ride on a user's earlier approval at the provider".

The MCP specification itself endorses the shape: the authorization server
"may be hosted with the resource server", and the Enterprise Managed
Authorization extension (SEP-990, implemented in go-sdk `auth/extauth`) is the
standardised variant where the MCP server's AS accepts an identity assertion
from the enterprise IdP. That variant needs IdP support for RFC 8693 token
exchange into ID-JAGs, which none of the five providers documents, so it is
not a shortcut today.

**What it costs in Go.** No Go library delivers an MCP-ready authorization
server. Building blocks:

- `ory/fosite`: "The security first OAuth2 & OpenID Connect framework for Go";
  authorization code, refresh, client credentials, PKCE (RFC 7636), opaque
  access tokens by default with an optional stateless JWT strategy (no
  revocation), introspection and revocation handlers. "Fosite does not ship a
  storage implementation"; login, consent and session management are the
  implementer's. DCR and CIMD are not mentioned. Source:
  <https://github.com/ory/fosite>.
- `zitadel/oidc`: "client (RP) and server (OP) implementation for the OIDC
  standard written for Go"; OP side lists Code Flow, PKCE, Discovery, JWT
  Profile; example servers include "some very basic login UI". Storage
  interface and DCR/CIMD support not stated in the README. Source:
  <https://github.com/zitadel/oidc>.
- `modelcontextprotocol/go-sdk`: the resource-server half
  (`auth.RequireBearerToken`, `auth.ProtectedResourceMetadataHandler`,
  `oauthex.MatchesResource`) and `oauthex` types for AS metadata, PRM, DCR
  requests and responses that can be reused on the server side. No AS.
- Standard library plus a JWT library (`golang.org/x/oauth2` for the upstream
  OIDC client; `github.com/coreos/go-oidc` or similar for ID-token
  verification; `github.com/go-jose/go-jose` or `github.com/golang-jwt/jwt`
  for signing). These are implementation options, not verified
  recommendations.

Components to write regardless of library (derived from the spec sections
above):

1. RFC 8414 metadata document with `code_challenge_methods_supported:
   ["S256"]`, `client_id_metadata_document_supported: true`,
   `registration_endpoint` (optional but what Cursor and today's Claude Code
   default to), `authorization_response_iss_parameter_supported: true`.
2. Client registration: fetch and validate CIMD documents (HTTPS, exact
   `client_id` match, redirect URI check, SSRF guard, caching); RFC 7591
   endpoint for DCR with `application_type` handling; a configured list of
   pre-registered clients.
3. `/authorize`: parse and validate the request (PKCE, `resource`,
   `redirect_uri` exact match), persist it with a state bound to the browser,
   redirect to the upstream OIDC provider with its own PKCE, handle the
   callback, verify the ID token, show consent naming the client, issue a code.
4. `/token`: code exchange with PKCE verification, refresh tokens with
   rotation for public clients, `resource`-bound audience, short-lived access
   tokens (signed JWT or opaque with a local verifier).
5. Storage for clients, pending authorizations, codes, tokens, consents.
6. Resource-server side: the verifier for the tokens issued above, the 401/403
   challenges, the PRM document pointing at this AS.
7. Operational: key generation and rotation, revocation endpoint (the
   Inspector calls RFC 7009 on `--relogin`), session cookie for the browser
   half, which overlaps with the open "Browser sessions" item in the map.

Rough size, judged from the scope of the spec pages and the two Go libraries'
READMEs rather than from any measured implementation: a few thousand lines of
Go plus tests, with the authorize/consent/upstream state machine being the
part that carries the most security weight. This is an estimate, not a
verified figure.

## Clients versus capabilities

| Client | OAuth flow unaided (PKCE, RFC 9728/8414 discovery) | CIMD | DCR | Pre-registered client id | Static bearer header | Redirect URI to allow-list | Source |
| - | - | - | - | - | - | - | - |
| Claude Code | Yes, via `/mcp` or `claude mcp login`; not in `-p` runs | Yes, auto-discovered | Yes, default | `--client-id`, `--client-secret`, `oauth` JSON | `--header`, `headersHelper` | `http://localhost:PORT/callback` | code.claude.com/docs/en/mcp |
| Claude Desktop / claude.ai | Yes, "Sign in now" or "when needed" | Yes, "Claude's published identity" (recommended) | Yes, "Register automatically" | Yes, "Use your own OAuth client" | Request headers (beta) | `https://claude.ai/api/mcp/auth_callback` (search summary only) | claude.com/docs/connectors/custom/add-unlisted |
| Cursor | Yes, PKCE, RFC 9728, RFC 8707 | Not documented | Yes, public client | `auth.CLIENT_ID`, `CLIENT_SECRET`, `scopes` | `headers` | `http://localhost:8787/callback`, `https://www.cursor.com/agents/mcp/oauth/callback` | cursor.com/docs/context/mcp |
| VS Code | Yes, browser on first connect | Yes since 1.106, preferred over DCR | Yes | `oauth.clientId` since 1.123, secret via CodeLens | `headers` with `${input:}` | Not stated in docs read | code.visualstudio.com updates v1_106, v1_123; agents/reference/mcp-configuration |
| MCP Inspector | Yes, web, TUI, CLI; `--stored-auth-only` for CI | Yes, install-wide `client.json`, user hosts the document | Yes | Per-server `clientId`/`clientSecret` | Per-server `headers` | `http://127.0.0.1:6276/oauth/callback` (CLI/TUI default) | github.com/modelcontextprotocol/inspector `488c30d` |
| Go SDK client | Library: discovery, PKCE check, `resource`, `iss`, step-up; browser and loopback are the caller's | Yes, `ClientIDMetadataDocumentConfig` | Yes, `DynamicClientRegistrationConfig` | Yes, `PreregisteredClient` | Caller's `http.Client`/headers | Caller-defined `RedirectURL` | go-sdk `0fa5e84` `auth/authorization_code.go` |

## Providers versus capabilities

| Provider | CIMD | DCR open to an unauthenticated MCP client | PKCE advertised in metadata | RFC 8414 document | Access token format | Audience the MCP server can verify | Source |
| - | - | - | - | - | - | - | - |
| Google | No (discovery doc) | No: no `registration_endpoint`; console only | Yes | Yes (200) | Opaque; `tokeninfo` only | No: `aud` is the OAuth client id | developers.google.com/identity/protocols/oauth2; docs.cloud.google.com/docs/authentication/token-types; live discovery doc |
| Okta | No (metadata example) | No: `okta.clients.register` scope required | Yes (`S256`) | Yes (custom AS) | RS256 JWT (custom AS); org AS tokens not for your apps | Yes, one configured audience per custom AS; API Access Management licence in production | developer.okta.com auth-servers, validate-access-tokens, createclient, customas metadata |
| Microsoft Entra | No (discovery doc) | No: "doesn't publish an RFC 7591 registration endpoint" | **No** (`code_challenge_methods_supported` absent, 2026-11-14) | No (404); OIDC discovery only | RS256 JWT (v1 or v2) | Yes: app registration with Application ID URI = server URL, v2 tokens, RFC 8707 `resource` matched | learn.microsoft.com secure-mcp-server-with-entra-id, access-tokens, plugin-authentication-dynamic-client-registration; live discovery doc |
| Keycloak | Not documented | Yes, if Trusted Hosts policy set (disabled by default); or initial access token | Not checked (no live instance) | Not checked | Signed JWT (JWS) | Yes, via Audience mapper or Audience Resolve on client scopes; resource server modelled as a client | keycloak.org securing-apps/client-registration, oidc-layers, server_admin "Audience support" |
| Authentik (2026.8+) | Not documented | No: bearer token with `goauthentik.io/oidc/dcr` scope required | Not checked | Not checked | Signed JWT (RS256 or HS256), optional JWE | No by default: `aud = provider.client_id`; token exchange can retarget | docs.goauthentik.io oauth2, dynamic-client-registration, token_exchange; `id_token.py` |

## What I could not verify from a primary source

- The claude.ai OAuth callback URL (`https://claude.ai/api/mcp/auth_callback`)
  and whether Claude Desktop differs in any way from claude.ai for custom
  connectors.
- Whether Cursor supports Client ID Metadata Documents.
- VS Code's redirect URI and its fallback to prompting for a client id when
  DCR fails (issue tracker only).
- Okta's help-centre statement that MCP servers need pre-registered clients
  (seen only in a search summary).
- RFC 8707 `resource` support at Google, Okta, Keycloak and Authentik.
- Keycloak and Authentik live discovery documents (`code_challenge_methods_supported`,
  `client_id_metadata_document_supported`), since no instance was available.
- Whether `zitadel/oidc` requires an implementer-supplied storage interface
  (README silent).
- The size estimate for a Go authorization server is judgement, not
  measurement.

## Sources

MCP specification (2026-07-28):

- <https://modelcontextprotocol.io/specification/versioning>
- <https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization>
- <https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/authorization-server-discovery>
- <https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/client-registration>
- <https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/security-considerations>

Go SDK:

- <https://github.com/modelcontextprotocol/go-sdk> at `0fa5e8407aa3fda65166e4207156c03d084bfd44`:
  `auth/client.go`, `auth/authorization_code.go`, `auth/auth.go`,
  `auth/extauth/enterprise_handler.go`, `oauthex/*.go`, `mcp/streamable.go`,
  `mcp/shared.go`, `examples/auth/{client,server,enterprise}/main.go`,
  `ROADMAP.md`

Clients:

- <https://code.claude.com/docs/en/mcp>
- <https://claude.com/docs/connectors/custom/add-unlisted>
- <https://cursor.com/docs/context/mcp>
- <https://code.visualstudio.com/updates/v1_106>
- <https://code.visualstudio.com/updates/v1_123>
- <https://code.visualstudio.com/docs/agents/reference/mcp-configuration>
- <https://github.com/modelcontextprotocol/inspector> at `488c30d`

Providers:

- <https://developers.google.com/identity/protocols/oauth2>
- <https://developers.google.com/identity/openid-connect/openid-connect>
- <https://docs.cloud.google.com/docs/authentication/token-types>
- <https://accounts.google.com/.well-known/openid-configuration> (live, 2026-11-14)
- <https://developer.okta.com/docs/concepts/auth-servers/>
- <https://developer.okta.com/docs/reference/api/oauth-clients/>
- <https://developer.okta.com/docs/api/openapi/okta-oauth/oauth/client/createclient>
- <https://developer.okta.com/docs/api/openapi/okta-oauth/oauth/customas/getwellknownoauthconfigurationcustomas>
- <https://developer.okta.com/docs/guides/validate-access-tokens/main/>
- <https://learn.microsoft.com/en-us/entra/identity-platform/access-tokens>
- <https://learn.microsoft.com/en-us/entra/agent-id/secure-mcp-server-with-entra-id>
- <https://learn.microsoft.com/en-us/microsoft-365/copilot/extensibility/plugin-authentication-dynamic-client-registration>
- <https://login.microsoftonline.com/common/v2.0/.well-known/openid-configuration> (live, 2026-11-14)
- <https://www.keycloak.org/securing-apps/client-registration>
- <https://www.keycloak.org/securing-apps/oidc-layers>
- <https://www.keycloak.org/docs/latest/server_admin/index.html> ("Audience support")
- <https://docs.goauthentik.io/add-secure-apps/providers/oauth2/>
- <https://docs.goauthentik.io/add-secure-apps/providers/oauth2/dynamic-client-registration>
- <https://docs.goauthentik.io/add-secure-apps/providers/oauth2/token_exchange/>
- <https://raw.githubusercontent.com/goauthentik/authentik/main/authentik/providers/oauth2/id_token.py>

Server-as-authorization-server pattern and Go libraries:

- <https://developers.cloudflare.com/agents/guides/remote-mcp-server/>
- <https://developers.cloudflare.com/agents/model-context-protocol/protocol/authorization/>
- <https://developers.cloudflare.com/agents/model-context-protocol/guides/securing-mcp-server/>
- <https://github.com/cloudflare/workers-oauth-provider>
- <https://github.com/ory/fosite>
- <https://github.com/zitadel/oidc>
