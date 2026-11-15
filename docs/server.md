# The Server

`ubiq serve` runs the Server: one long-running process that serves the glossary page and the MCP tools over HTTP for one
repository, with reads and writes. Every caller is a Person an external OpenID Connect provider has identified, and
every write is a Change: one commit, authored as that Person, in a clone the Server owns. This document is the design
as decided; the hard-to-reverse choices are ADRs 0005 to 0009 in [adr](adr), and the words are Terms in
[GLOSSARY.md](../GLOSSARY.md). Where the three disagree, the ADR wins over this document, and the Glossary wins on
what a word means.

Decisions are indexed on the map
[Served Ubiq: one process for the page and remote MCP, with writes, behind OpenID Connect](https://github.com/stefaneg/ubiq/issues/4);
each line there links the ticket that holds the reasoning.

## Scope

In: the Server, its transports and login, the Reader and Writer roles, the read and write tools, how a Change lands
and reaches the default branch, the Server's flags, what the Deployment configures around it, and how the first AWS
instance is produced from code.

Out, deliberately: hosting several organisations on one instance; public unauthenticated read (the static page from
`ubiq html` covers a public glossary); provider-specific login code, including a direct "log in with GitHub" (GitHub
offers no OpenID Connect login for users, so it needs a provider in front); write tools in the local stdio `ubiq mcp`,
which stays read-only because a write there would be an uncommitted working-tree edit, not a Change; write tools for
Context Relationships and new Contexts, which are structural work a Maintainer does by hand; authoring from the web
page, since the Agent is the write client and the page stays a rendering; marking on the page which Terms sit in a
Batch not yet merged, so an unmerged Term looks like any other; and production readiness: high availability,
monitoring, backups, rate limits, and a pipeline that runs the provisioning. Infrastructure as code is in, the
pipeline is not.

## Shape

One Deployment is one small EC2 instance running two systemd units: Pomerium, an identity-aware proxy, holding ports
80 and 443, and the Server listening on loopback only
([ADR 0007](adr/0007-the-server-runs-beside-its-proxy-on-one-instance.md)). The proxy owns three hostnames under one
domain: its own authenticate service, the page route and the MCP route. The Server has no notion of its public URL,
no TLS, and no authentication code of its own
([ADR 0006](adr/0006-an-identity-aware-proxy-authenticates-served-ubiq.md)).

```
browser ──┐                 ┌──────────── Pomerium ────────────┐        ┌──── ubiq serve ────┐
          ├─ https ────────▶│ authenticate.  ubiq.   mcp.      │─ http ▶│ /       the page   │
Agent ────┘                 │ (login, OAuth 2.1 for Agents)    │ + JWT  │ /mcp    the tools  │
                            └──────────────────────────────────┘        └──────┬─────────────┘
                                                                               │ git fetch / push
                                     Identity Provider                      origin, Batch ubiq/<n>
```

The local stdio `ubiq mcp` is unchanged: a second transport for the same read tools, unauthenticated, reading the
working tree it is started in. Served mode is an addition, not a replacement.

## Who is calling

Every request to either route is behind login; there are no local accounts and no anonymous path. Pomerium logs the
Person in at the Deployment's Identity Provider, found by standard OpenID Connect discovery, so any provider works and
none gets provider-specific code.

**A browser** asking for the page is redirected to the Identity Provider's own login form and back; the page then
shows the Person. **An Agent** connecting to the MCP route gets `401` with a pointer to the route's protected-resource
metadata, discovers Pomerium as the OAuth 2.1 authorization server, registers (by a client ID metadata document, as
Claude Code does, or by dynamic registration when the Deployment turns it on), and runs the authorization-code flow
with PKCE; the browser that opens shows the same login form. Access tokens last fourteen hours and come with a refresh
token. Browser and Agent therefore arrive as the same Person through the same login, and an Agent's Change is
attributable to the Person driving it.

On both routes Pomerium forwards the Person to the Server as a signed assertion JWT in the `X-Pomerium-Jwt-Assertion`
header. The Server verifies it against the keys the proxy publishes (the `-assertion-keys` URL) and reads from it the
Person's subject, name, email and the claim the Writer Rule names. A request without a valid assertion is answered
`401`: the Server never guesses a Person. Swapping proxies later means swapping this one verifier.

**Roles.** The Server itself tells a Writer from a Reader by the Writer Rule, one claim name and its accepted values
from the Deployment's configuration, `groups` by default
([ADR 0008](adr/0008-the-server-enforces-the-writer-rule-itself.md)). A Person whose assertion carries the claim with
an accepted value is a Writer; everyone else the Identity Provider admitted is a Reader. The proxy may add its own
policy as a second gate, but nothing relies on it. The Server never answers Maintainer: it cannot tell who may merge,
so a Maintainer who logs in is a Writer or a Reader like anyone. The rule lives in the Deployment and not in the
repository because the Server reads the Domain Model from the open Batch, and content must not govern its own
authorisation.

## What the Server reads

The Server owns a clone of the repository at `-root`, cloned from `-origin` on first start when the directory holds
no `.git`, and it reads from and writes to the **open Batch**, a shared branch `ubiq/<n>`, never from the default
branch and never from any developer's working tree
([ADR 0005](adr/0005-changes-land-in-a-numbered-batch-branch.md)). So the page and every Lookup show the language as
the team is currently writing it, Changes awaiting merge included, and an Agent sees its own Change on its next
Lookup. This checkout exists nowhere else: an Agent that compares the Server's answers with the branches on its own
machine will find differences. The MCP server's instructions therefore say which branch the Server reads, and every
write result names the Batch it landed in.

Every call reads the Domain Model afresh from the clone; nothing is cached
([ADR 0003](adr/0003-read-the-domain-model-on-every-call.md)).

**The page** at `/` is the `ubiq html` page, Health Check included, with the Person's name and role at the top. No
separate routes.

**The read tools** are one set, identical in stdio `ubiq mcp` and on the Server, with the same input and output:

| Tool | Input | Answer |
|---|---|---|
| `lookup` | `words`, a list | For each word, what `ubiq lookup` prints. The Term header line is `Name (Context / Cluster), path:line`; the Cluster is left out when the Term sits under none. |
| `check` | `text` | What `ubiq check` prints: the Terms and Avoided Words the text uses and its Candidate Phrases. |
| `health` | none | The whole Health Check as `ubiq health` prints it, one `path:line: rule: name: note` line per Finding and the summary line. |
| `whoami` | none | On the Server: the Person's name, email and role, Reader or Writer, nothing about claims or the proxy. In stdio: the git identity of the working tree (`user.name`, `user.email`, or "unset") with role Maintainer. |

## Writes

The Server offers four write tools, one per domain operation. The Agent never sees Markdown: it writes plain text and
the Server lays the entry out. Writes are refused for a closed list of reasons and otherwise land, Findings included
([ADR 0009](adr/0009-a-change-lands-with-its-findings.md)).

| Tool | Input | Effect |
|---|---|---|
| `define_term` | `name`, `definition` (plain text), `avoid` (list, optional), `cluster` (optional), `context` (optional; required when the repository has more than one Context) | A new Term entry in the Glossary. |
| `revise_term` | the same fields, plus `new_name` (optional) | Full replacement of the definition and the avoid list; the Agent looked the Term up first, there are no patch operations. `cluster` moves the Term. `new_name` renames it and rewrites every Reference to it in the same Change. |
| `remove_term` | `name`, `context` as above | Removes the entry. |
| `record_adr` | `title`, `text` (free paragraphs) | A new file in the repository's `docs/adr`; the Server picks the next number and the slug and returns the path. No status or supersedes fields. Per-Context ADR directories are not written in this spec. |

**Layout the Server applies.** In a definition, every exact match of an existing Term's name, or its plural, is
bolded, longest name first; everything else stays as typed, so a proper noun such as "OpenID Connect" is possible and
never refused. This happens at define and revise time only: a later Term never rewrites earlier definitions. A Term
goes at the end of its Cluster; with no Cluster named, after the last Term of the Glossary; an unknown Cluster is
created after the last Term and before the next `##` section. Names are matched without regard to case.

**Findings warn, never refuse.** A Change that introduces an Orphan, Unresolved Phrase, Long Definition or Unmapped
Name lands, and the result lists the Findings the Change introduced, not the whole Health Check. Removing a referenced
Term lands and reports each Unresolved Phrase it left behind. Pre-existing Findings never matter to a write; an Agent
cleaning up calls `health`.

**The closed refusal list.** A write is refused when, and only when:

- the Person is a Reader, in which case the refusal names the Writer Rule's claim and accepted values;
- `define_term` names a Term that exists in that Context, or `revise_term` or `remove_term` names one that does not;
- `context` names no Context;
- `name`, `definition`, `title` or `text` is empty;
- the Batch is in conflict with the default branch (see below), in which case the refusal names the branch and the
  conflicting files.

A refusal is one sentence that names only tools the Server lists, so an Agent can act on it. The write tools stay
listed for a Reader; only the call is refused.

**The result of a write** is the Term or ADR as it now reads, the commit id, the Batch name, and the Findings
introduced.

**The commit.** One Change is one commit with subject `Define Term <name>`, `Revise Term <name>`,
`Remove Term <name>` or `Record ADR <title>` and no body. Its author is the Person, name and email from the assertion;
its committer is the Server's own identity from `-committer`. Changes are serialised in the one Server process, so a
stale revise simply wins last; history keeps the overwritten text. There is no precondition field.

## Batches

Origin is the only source of truth and the clone is a cache; the Server keeps no state that exists only in the clone.
Before every Change the Server fetches origin and finds the open Batch: the `ubiq/<n>` branch with the highest `n` on
origin, or `ubiq/1` from the default branch when there is none. The default branch is the one origin's `HEAD` points
to; the prefix `ubiq/` is fixed.

On every fetch the Server merges the default branch into the open Batch, so developers' edits on the default branch
appear on the page and in Lookups without waiting for anyone. Then:

- When that merge **conflicts**, the Change is refused, naming the Batch and the conflicting files, until a
  Maintainer resolves it on the branch. Reads keep serving the clone as it stands. A conflict found at startup is
  logged and is not fatal: reads continue, writes refuse.
- When the Batch **holds a Change and differs from the default branch in nothing**, a Maintainer has merged it, by
  fast-forward, merge commit, squash or rebase alike. The next Change opens `ubiq/<n+1>` from the default branch.
  "Holds a Change" means "has a commit the Server committed": a Batch opened by hand with no Change is never taken as
  merged, and a merged Batch whose history was kept is not mistaken for an empty one.
- Otherwise the Change is committed on the Batch and pushed straight after. A push rejected because the branch moved
  is retried after a fetch.

A Maintainer merges each Batch into the default branch by hand, opening a pull request or merge request if the
repository wants one. The Server never calls a forge API, so a protected default branch is no concern of its. Against
a local origin the whole fetch, merge, edit, commit and push takes about a quarter of a second.

## Configuring the Server

Every setting is a flag on `ubiq serve`, like every other ubiq command: no configuration file, no environment
variables. The Deployment's configuration is the systemd unit's `ExecStart` line. No secret lives on the Server's
side; the push credential is git's (next section).

| Flag | Default | Meaning |
|---|---|---|
| `-root <dir>` | `.` | The clone the Server owns. Cloned from `-origin` when it holds no `.git`, reused otherwise. |
| `-origin <https url>` | required | The repository. HTTPS only; the push token cannot serve SSH. |
| `-listen <addr>` | `127.0.0.1:8766` | Loopback; the proxy sits in front. |
| `-assertion-keys <url>` | required unless `-person` | Where the proxy publishes the keys that sign its assertion. |
| `-ca <file>` | empty, system roots | Extra CA to trust when fetching the keys. Dev passes `dev/certs/ca.pem`. |
| `-writer-claim <name>` | `groups` | The Writer Rule's claim. |
| `-writers <v,v,...>` | required | The Writer Rule's accepted values. |
| `-committer "Name <email>"` | `ubiq serve <ubiq@localhost>` | The Server's git identity, committer of every Change. Set to the push account. |
| `-person "Name <email>"` | off | Dev direct mode: a fixed Person who is a Writer, no assertion verification. |

A worked example, the unit's line in production:

```
ExecStart=/usr/local/bin/ubiq serve \
  -root /var/lib/ubiq/repo \
  -origin https://github.com/example/handbook.git \
  -assertion-keys https://authenticate.glossary.example.com/.well-known/pomerium/jwks.json \
  -writer-claim groups -writers glossary-writers \
  -committer "ubiq-bot <ubiq-bot@example.com>"
```

The Server **refuses to start** when `-writers` is missing (missing means nobody may write, not that everyone may),
when neither or both of `-assertion-keys` and `-person` are given, when `-person` is given and `-listen` is not a
loopback address, or when `-origin` is an SSH URL. Logging is one line per request and per Change to stderr, with no
flags; journald keeps it under systemd, Docker in development.

**Dev direct mode.** With `-person` the Server runs alone, without a proxy, and every request is that Person, a
Writer; `-origin` is still required and points at a local bare repository. It exists so a developer can exercise reads
and writes in one command. Reader behaviour, and the login flows, are exercised only behind the development proxy.

## What the Deployment owns

**The proxy's settings.** Pomerium's configuration belongs to the Deployment, not to ubiq. The reference file is
produced with provisioning ([deploy/packer](../deploy/packer)); this is what it must say and why. Development values
are those of the stack in [dev](../dev).

| Setting | Why | Development value |
|---|---|---|
| `authenticate_service_url` | The Deployment's own authenticate service, not Pomerium's hosted default. | `https://authenticate.localhost.pomerium.io` |
| `certificate_file`, `certificate_key_file`, or `autocert` | TLS is the proxy's. | A throwaway wildcard certificate; production uses `autocert`. |
| `shared_secret`, `cookie_secret` | Pomerium's own; generated at first boot in production. | Fixed in the dev file, never reused. |
| `databroker_storage_type`, `databroker_storage_connection_string` | Sessions and registered clients; a file on the instance, lost when it is replaced. | `file`, under the container's data directory. |
| `idp_provider: oidc`, `idp_provider_url`, `idp_client_id`, `idp_client_secret` | The Identity Provider by discovery. | Keycloak realm `ubiq`; Google in the test Deployment. |
| Route `ubiq.<domain>` → the Server, `pass_identity_headers: true` | The page; the assertion must reach the Server. | `http://host.docker.internal:8766` |
| Route `mcp.<domain>` → the Server, `pass_identity_headers: true`, `mcp: server` | The MCP endpoint; `mcp: server` makes the route an OAuth 2.1 protected resource with Pomerium as its authorization server. Bearer tokens only; a browser cookie does not open it. | the same upstream |
| Policy on both routes: `authenticated_user: true` | A Reader is anyone the Identity Provider admits; the Writer Rule does the rest. | Replaces the dev stack's narrower `domain: is: ubiq.test`. |
| `mcp_allowed_client_id_domains` | Clients registering by a client ID metadata document must come from a listed host. | `[claude.ai]` |
| Runtime flag `mcp_dynamic_client_registration` | For clients without a metadata document; off by default, and the metadata then shows no registration endpoint. | `true` |

Two facts the Server accommodates: Pomerium forwards the upstream's `Host`, so the Server turns off the MCP SDK's
DNS-rebinding guard on its loopback listener; and Pomerium probes the Server for OAuth metadata on every authorize and
logs a warning about plain `http`, which is harmless.

**The push credential.** The Server runs the host's `git` and nothing else; it knows no git host and holds no token.
The service user, `ubiq`, has `credential.helper store` and one line in `~/.git-credentials`,
`https://<account>:<token>@<host>`, mode `0600`, written by provisioning. git reads it on every push, so rotating the
token is replacing that line with no restart. The token is scoped to the one repository: on GitHub, a fine-grained
personal access token on a dedicated machine account that is a collaborator with contents read and write; on GitLab, a
project access token with `write_repository`, whose bot user GitLab creates. Any HTTPS git host with a token works.
`-committer` names that account or bot, so the push and the committer agree.

## Going to AWS

Packer bakes and Terraform places, both run from a Maintainer's machine; there is no pipeline
([ADR 0007](adr/0007-the-server-runs-beside-its-proxy-on-one-instance.md)). The code lives in
[deploy/packer](../deploy/packer) and [deploy/terraform](../deploy/terraform) and is not reproduced here.

**Packer** bakes an Amazon Linux 2023 arm64 image for a `t4g.micro`: Pomerium from its rpm repository, which brings
its own unit and user; the ubiq binary cross-compiled on the Maintainer's machine (`GOOS=linux GOARCH=arm64`) until
the repository has releases; the Server's unit, the Pomerium configuration template and a first-boot script. ubiq is
never built on the instance.

**Terraform** places one instance: an Elastic IP, a security group with ports 80 and 443 in and nothing else, an
instance role with SSM Session Manager (no SSH, no key pair, port 22 closed), and user-data rendered from variables
that lands as `/etc/ubiq/provision.env`: domain, Identity Provider issuer and client id and secret, Writer claim and
values, origin URL, push account and token, committer. First boot renders Pomerium's configuration from it, generates
the shared and cookie secrets, writes the `ubiq` user's credentials, and starts both units. The clone is at
`/var/lib/ubiq/repo`. Terraform state is local and gitignored.

**The `domain` variable** has two modes. Empty, Terraform derives `<eip-with-dashes>.sslip.io`, free public DNS under
which `authenticate.`, `ubiq.` and `mcp.` all resolve to the Elastic IP, so testing needs no domain. Set, it is the
real domain, and Terraform writes the wildcard record only when a Route53 zone is also given; otherwise the record is a
manual step. Pomerium's `autocert` fetches Let's Encrypt certificates for the three names either way. The instance's
default `amazonaws.com` name was rejected: Let's Encrypt refuses those names by policy, and one hostname cannot carry
the authenticate service and two routes.

**The instance is disposable.** An upgrade and a credential rotation are the same act: change the input,
`packer build` if the binary changed, `terraform apply`, and the instance is replaced. The Server re-clones from
origin on first start and the Batch lives on origin, so nothing is lost but Pomerium's sessions: everyone logs in
again. Secrets ride in user-data, readable by anyone with EC2 describe rights on the account; that is accepted for
this scope, and SSM Parameter Store is the hardening step.

**The test Deployment** uses Google as the Identity Provider through the generic `oidc` provider with issuer
`https://accounts.google.com`. Google sends no `groups`, so the Writer Rule there is `-writer-claim email -writers
<address>`: the worked example that the rule is not tied to groups. Production uses the organisation's provider.

**By hand, in this order:**

1. Create the Identity Provider's OAuth client (for Google, in the Cloud console) with the authenticate service's
   callback URL.
2. Create the push credential: a GitHub machine account as collaborator with write on the one repository and a
   fine-grained token with contents read and write, or a GitLab project access token with `write_repository`; the
   longest lifetime the host allows, and a calendar reminder for its expiry.
3. Optionally, the DNS record, when a domain is used without a Route53 zone.
4. `packer build`, then `terraform apply` with the variables above.
5. Log in to the page; connect Claude Code to the MCP route; `whoami`; then `define_term` as the smoke test, once
   against GitHub and once against GitLab.

## Development

The stack in [dev](../dev) runs Pomerium and Keycloak under Docker Compose with everything on loopback, two test
people (`writer` in group `ubiq-writers`, `reader` in none), curl scripts that play a browser and an MCP client, and a
README of facts observed. It was built around the prototype; implementation makes it match this document: the Server's
`-jwks` becomes `-assertion-keys`, `-default` goes, and the routes' admission becomes `authenticated_user: true`.

## Decisions

| ADR | Decides |
|---|---|
| [0003 Read the Domain Model on every call](adr/0003-read-the-domain-model-on-every-call.md) | No cache; every call reads the files. |
| [0005 Changes land in a numbered Batch branch](adr/0005-changes-land-in-a-numbered-batch-branch.md) | The open Batch, merged by a Maintainer by hand. |
| [0006 An identity-aware proxy authenticates served ubiq](adr/0006-an-identity-aware-proxy-authenticates-served-ubiq.md) | No auth code in ubiq; Pomerium first. |
| [0007 The Server runs beside its proxy on one instance](adr/0007-the-server-runs-beside-its-proxy-on-one-instance.md) | One EC2 instance, two units, Packer and Terraform. |
| [0008 The Server enforces the Writer Rule itself](adr/0008-the-server-enforces-the-writer-rule-itself.md) | Roles from one claim, decided in ubiq. |
| [0009 A Change lands with its Findings](adr/0009-a-change-lands-with-its-findings.md) | Findings warn; a closed refusal list. |

The Terms Server, Deployment, Identity Provider, Person, Agent, Reader, Writer, Writer Rule, Change, Batch and
Maintainer are in [GLOSSARY.md](../GLOSSARY.md) under Serving, People and Changes. The proxy, its assertion, the
service user, the token and the image are implementation, not Terms.
