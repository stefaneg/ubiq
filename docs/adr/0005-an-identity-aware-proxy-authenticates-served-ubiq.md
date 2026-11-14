# An identity-aware proxy authenticates served ubiq

A served ubiq owns no authentication code. It sits behind an identity-aware proxy, Pomerium first, which logs each
Person in at the deployment's Identity Provider, acts as the OAuth 2.1 authorization server that Agents talk to, and
hands ubiq the Person as a signed assertion that ubiq verifies against the proxy's published keys. ubiq listens only
where the proxy can reach it. An ordinary Identity Provider cannot serve Agents directly: none of the common ones lets
an MCP client register itself, and most issue tokens whose audience ubiq cannot verify. Building the authorization
server into ubiq would work, but it is a few thousand lines of security-critical code whose authorize, consent and
upstream-login state machine a glossary tool should not carry.

## Considered options

- ubiq as its own authorization server, brokering to the Identity Provider: kept as the fallback if the proxy
  disappoints.
- Agents obtaining tokens from the Identity Provider directly, ubiq only verifying: rejected, see above.

## Consequences

- Browser and Agent arrive as the same Person through the same login, and ubiq's roles and commit identity come from
  the assertion's claims.
- TLS and certificates are the proxy's, not ubiq's.
- A deployment must point the proxy's authenticate service at itself rather than Pomerium's hosted default.
- Swapping proxies later means swapping one assertion verifier.
