# Lambda or EC2 for a server that owns a git clone

Research for issue #7, part of map #4 (served Ubiq). Checked 2026-11-14 against
AWS documentation and pricing, the Go Lambda libraries, the MCP specification and
the Go MCP SDK that Ubiq already depends on. Prices are US East (N. Virginia),
on-demand, USD, taken from the AWS Price List API where the pricing pages load
their tables dynamically. Every claim cites its source in [Sources](#sources);
things that could not be confirmed from a primary source are listed in
[Not verified](#not-verified).

## Short answer

**A small EC2 instance is the simplest thing that works. Lambda is possible, but
only in a shape that fights the design, and the shape that would fit the design
(a persistent clone on EFS) costs more than the instance and disables response
streaming.**

- Lambda's filesystem is per execution environment and is a cache, not a store.
  `/tmp` survives between invocations only while that one environment is kept
  warm; Lambda terminates environments "every few hours" and starts a fresh
  empty one for every concurrent request. A clone in `/tmp` must be re-fetched on
  every cold start, and two concurrent writers would each hold their own clone.
  [L2][L13]
- The persistent alternative, EFS, requires the function to join a VPC. A
  VPC-attached function has no internet access without a NAT gateway (needed to
  reach the identity provider and to push to the git host), and NAT alone costs
  about $0.045/hour, roughly $33/month, more than the whole EC2 option. Function
  URLs also "do not support response streaming within a VPC environment". [L3][L4][L5][V2]
- Streamable HTTP MCP survives on Lambda only in **stateless** mode: no
  `Mcp-Session-Id`, no server-initiated GET stream, each POST self-contained.
  The MCP spec allows this (sessions are MAY, GET may return 405), and the Go SDK
  Ubiq uses has a `Stateless` switch for exactly this. Stateful sessions held in
  one environment's memory break as soon as a request lands elsewhere. [M1][M2]
- An EC2 `t4g.micro` (2 vCPU, 1 GiB, arm64) is $0.0084/hour, about $6.13/month,
  plus $0.005/hour for its public IPv4 (about $3.65/month) and a few cents of EBS.
  Total around $10–11/month with no other moving parts: one Go binary, Caddy (or
  the Go binary itself) terminating TLS with a free Let's Encrypt certificate, one
  DNS A record, one clone on local disk, one process-wide lock around
  pull-commit-push. [E1][E2][E4][T1]
- Lambda at this traffic level would be nearly free in compute (the free tier is
  1M requests and 400,000 GB-seconds a month, and function URLs cost nothing
  extra), but the saving is bought with re-cloning on cold start, a bundled or
  pure-Go `git`, a random `*.lambda-url.*.on.aws` hostname unless CloudFront is
  added, and a 15-minute hard cap on any one HTTP response. [L7][L9][L13]

Recommendation for the spec: **EC2, one instance, Caddy in front, clone on the
root volume.** Keep Lambda out of the ADR except as the rejected alternative,
with the reasons above.

## 1. Lambda's filesystem

### Ephemeral storage (`/tmp`)

- Size: "You can configure ephemeral storage between 512 MB and 10,240 MB, in
  1-MB increments." 512 MB is included; above that costs $0.0000000309 per
  GB-second (same for arm64). [L1][L13]
- Scope: "This storage is temporary and unique to each execution environment."
  [L1]
- Lifetime: "The directory content remains when the execution environment is
  frozen, providing a transient cache that can be used for multiple
  invocations." But: "Lambda terminates execution environments every few hours
  to allow for runtime updates and maintenance—even for functions that are
  invoked continuously. You should not assume that the execution environment
  will persist indefinitely." [L2]
- After a crash or timeout Lambda resets the environment but "does not clear the
  `/tmp` directory content before the next init phase". [L2]
- Concurrency: "For each concurrent request, Lambda provisions a separate
  instance of your execution environment." Each environment handles one request
  at a time. So two overlapping writes run in two environments with two
  different `/tmp` clones. [L6]

**Consequence.** A clone in `/tmp` is viable only if the design treats the
remote as the single source of truth: on first use in an environment, clone;
before every write, fetch/pull; after the commit, push immediately; and
serialise writes across environments either by setting reserved concurrency to
1 (which also caps the function at 10 requests/second and returns HTTP 429 above
that) or by accepting push rejections and retrying. [L6][L7]

### EFS as a mount

- A function "can mount an Amazon EFS file system to a local directory", at a
  path "starting with `/mnt/`", through an access point. The file system is
  shared and persistent: "your function code can access and modify shared
  resources safely and at high concurrency." [L3]
- Setup: the function must be attached to a VPC; "Create a file system in Amazon
  EFS with a mount target in every Availability Zone that your function connects
  to. For performance and resilience, use at least two Availability Zones";
  security groups must allow NFS (port 2049); the execution role needs
  `elasticfilesystem:ClientMount` / `ClientWrite` plus the VPC permissions. [L3]
- Internet from a VPC function: "When you attach your function to a VPC, it can
  only access resources available within that VPC. To give your function access
  to the internet, you also need to configure the VPC to have internet access."
  Connecting to a public subnet "doesn't give it internet access or a public IP
  address." That means a NAT gateway (or an equivalent) for OIDC discovery,
  token exchange and `git push`. [L4]
- Streaming in a VPC: "Lambda function URLs do not support response streaming
  within a VPC environment." [L5]
- Latency: EFS General Purpose, Regional, Elastic throughput: "~1 millisecond"
  reads, "~2.7 milliseconds" writes, best case. Git operations are many small
  metadata calls, so a status/commit on EFS is noticeably slower than local
  disk; no primary source quantifies git on EFS specifically. [F1]
- Cost: Standard storage $0.30 per GB-month (One Zone $0.16); Elastic throughput
  $0.03 per GB read and $0.06 per GB written. For a clone of a few hundred files
  this is cents. The expensive part is the NAT gateway: the VPC pricing page's
  worked example gives $0.045 per hour plus $0.045 per GB processed (quoted for
  US East (Ohio); N. Virginia rate not shown on the page). [F2][V2]

### Other Lambda shapes checked

- **Lambda Web Functions** run an ordinary HTTP server that listens on a port,
  with many concurrent requests per environment and built-in streaming, but
  "currently support Node.js runtimes only." Not usable for Go. [L10]
- **Lambda Managed Instances** run functions on EC2 capacity in your account
  with "a 15% management fee on top of the EC2 instance cost" and launch "three
  instances by default". Strictly more expensive and more complex than one EC2
  instance. [L11]
- **Container images** (up to 10 GB uncompressed) are the practical way to ship a
  `git` binary alongside the Go server if Lambda is chosen. [L7]

## 2. Cold starts when the clone must be fetched

- Lambda's own numbers: "Cold starts typically occur in under 1% of
  invocations. The duration of a cold start varies from under 100 ms to over 1
  second." That is for the runtime and code download; a git clone is extra and
  not covered by AWS. [L2]
- The `Init` phase (code outside the handler) "is limited to 10 seconds. If all
  three tasks do not complete within 10 seconds, Lambda retries the `Init` phase
  at the time of the first function invocation with the configured function
  timeout." Cloning inside the handler on first use avoids that limit; cloning
  in `init()` risks a retry. [L2]
- Bandwidth per execution environment is 625 Mbps; a repository of a few
  hundred Markdown files is a few MB, so the clone is bound by round-trips to
  the git host, not throughput. No AWS source gives a clone timing; see
  [Not verified](#not-verified). [L7]
- Cold-start time is billed: "You are charged for this time, and it adds latency
  to your overall invocation duration." [L2]
- The usual mitigation, provisioned concurrency, costs $0.0000097222 per
  GB-second (x86) / $0.0000077778 (arm64) continuously, which at 512 MB is about
  $12.80 (x86) / $10.24 (arm64) a month per warm environment, in the same range as
  the EC2 instance and still a cache, not a store. [L13]
- `git` availability: `provided.al2023` is "less than 40MB" and ships "no
  pre-installed language runtime"; whether it includes a `git` binary is not
  stated. Either bundle git in a container image, or use a pure-Go
  implementation such as go-git ("a highly extensible git implementation library
  written in pure Go", which "aims to be fully compatible with git"). Ubiq's
  `go.mod` today has no git dependency. [L12][G1]

On EC2 none of this applies: the clone is made once at provisioning and lives on
the root volume.

## 3. TLS and a public URL, and what it means for OpenID Connect

OpenID Connect requires the redirect URI to "exactly match one of the Redirection
URI values for the Client pre-registered at the OpenID Provider", and it "SHOULD
use the https scheme". So the public base URL must be known and stable before
the client is registered, and it must be HTTPS. [O1]

### Lambda function URL

- Format `https://<url-id>.lambda-url.<region>.on.aws`; "Once you create a
  function URL, its URL endpoint never changes." HTTPS is built in; dual stack.
  Auth type `NONE` (the server does its own OIDC login) or `AWS_IAM` (SigV4,
  unusable by browsers). No extra charge. [L8]
- No custom domain. AWS lists "custom domain names" among the reasons to choose
  API Gateway over function URLs. The way to put your own name in front is a
  CloudFront distribution with the function URL as origin; with origin access
  control the function URL must use `AWS_IAM` and then "If you use `PUT` or
  `POST` methods with your Lambda function URL, your users must compute the
  SHA256 of the body and include the payload hash value of the request body in
  the `x-amz-content-sha256` header", which browsers and MCP clients will not
  do, so the realistic CloudFront setup leaves the function URL public
  (`NONE`, "Do not sign requests"). CloudFront's Free plan is $0 per
  distribution per month with 100 GB and 1M requests. [L9][C1][C2]
- OIDC consequence: register `https://<url-id>.lambda-url.<region>.on.aws/callback`
  (random but stable), or add CloudFront + Route 53 ($0.50 per hosted zone per
  month) for a readable name. Deleting and recreating the function URL changes
  the hostname and breaks the registration. [L8][R1]

### API Gateway in front of Lambda

- HTTP API: "Maximum integration timeout 30 seconds", not adjustable; payload
  10 MB. REST API: "50 milliseconds - 29 seconds" default, raisable for Regional
  and private APIs "but this might require a reduction in your Region-level
  throttle quota"; payload 10 MB; idle connection timeout 310 seconds. [A1][A2]
- Custom domain names are supported directly (120 per account per Region). [A1][A2]
- Response streaming only through the REST API Lambda proxy integration using
  `InvokeWithResponseStream`, with a specific metadata-plus-delimiter output
  format. [A3]
- Price: REST $3.50 per million calls at the first tier (N. Virginia); HTTP API
  examples show $1.00 per million. Free tier of 1M calls per month for 12 months. [A4]

### EC2

- Public address: every public IPv4 is $0.005 per hour "In-use" or "Idle"
  (about $3.65/month). An Elastic IP keeps it stable across stop/start at the
  same price. [E2]
- TLS on the instance: Caddy "provisions TLS certificates for all your sites and
  keeps them renewed" from "a public ACME CA such as Let's Encrypt or ZeroSSL",
  needs the A/AAAA record pointing at the server and "ports 80 and 443 are open
  externally", and "redirects HTTP (default port 80) to HTTPS (default port 443)
  automatically." No AWS service required. [T1]
- TLS via AWS instead: ACM public certificates are free "for use with services
  integrated with ACM", which means an Application Load Balancer or CloudFront,
  not the instance itself. An ALB is $0.0225 per hour (about $16.43/month) plus
  LCUs, more than the instance; not the simplest path. [E3][E5]
- DNS: Route 53 hosted zone $0.50/month, or any registrar's DNS. [R1]
- OIDC consequence: the redirect URI is `https://<your-name>/callback` from day
  one and never depends on an AWS-generated identifier.

## 4. A Go HTTP server with streamable HTTP MCP on Lambda

### What the MCP transport requires

- One endpoint supporting POST and GET. Every client message is a new POST; the
  server answers each request with either `application/json` or an SSE stream
  that "SHOULD" close after the response. [M1]
- GET opens a server-to-client SSE stream, but the server may instead "return
  HTTP 405 Method Not Allowed, indicating that the server does not offer an SSE
  stream at this endpoint." [M1]
- Sessions are optional: the server "MAY assign a session ID at initialization
  time". If it does, clients "MUST include it" on every request, and the server
  "MUST respond to requests containing that session ID with HTTP 404 Not Found"
  once the session is gone, after which the client "MUST start a new session".
  [M1]

### How Ubiq's SDK maps onto that

`github.com/modelcontextprotocol/go-sdk` v1.8.0, already in `go.mod`:

- `StreamableHTTPHandler` keeps `sessions map[string]*sessionInfo` in process
  memory; an unknown `Mcp-Session-Id` gets 404. On Lambda that memory is one
  execution environment, so any request routed to a different environment, or
  after a recycle, forces the client to re-initialise. [M2]
- `StreamableHTTPOptions.Stateless`: "A stateless server does not read or set
  the Mcp-Session-Id header, and uses a temporary session with default
  initialization parameters for each request." "Any server->client request is
  rejected immediately as there's no way for the client to respond." "In
  Stateless mode, GET and DELETE requests return 405 Method Not Allowed." This
  is the mode that works on Lambda, and it is spec-compliant. Ubiq's tools are
  request/response (`lookup`, `check`, future writes), so losing server-initiated
  requests costs nothing today. [M1][M2]
- `JSONResponse` makes POST responses `application/json` instead of SSE, which
  removes the need for response streaming on Lambda altogether. [M2]

### Adapters for running `net/http` on Lambda

- `github.com/aws/aws-lambda-go/lambdaurl`: "serves requests from Lambda
  Function URLs using http.Handler"; `Start(handler http.Handler, ...)`. "Only
  Lambda Function URLs configured with `InvokeMode: RESPONSE_STREAM` are
  supported", on the `provided` runtimes. Go binary is named `bootstrap`,
  runtime `provided.al2023` (arm64 supported). [G2][G3][L12]
- AWS Lambda Web Adapter: a layer (`LambdaAdapterLayerArm64` / `X86`) or a
  binary copied into a container image that starts your real HTTP server on
  `AWS_LWA_PORT` (default 8080), waits for a readiness check, and proxies
  invocations from "Amazon API Gateway Rest API and Http API endpoints, Lambda
  Function URLs, and Application Load Balancer". `AWS_LWA_INVOKE_MODE` is
  `buffered` or `response_stream`. The README lists Go (Gin) examples. It does
  not state SSE support explicitly; see [Not verified](#not-verified). [G4]
- Response streaming on Lambda is native only for Node.js; Go goes through the
  Runtime API streaming mode (`Lambda-Runtime-Function-Response-Mode: streaming`,
  chunked transfer), which is what `lambdaurl` and the Web Adapter implement. [L5][L14]

### Limits on long-lived connections

- Function timeout max 900 seconds; one environment serves one request at a
  time; "Streaming responses incur cost and streamed responses are not
  interrupted or stopped when the invoking client connection is broken.
  Customers are billed for the full function duration". A long-lived SSE GET
  stream therefore occupies a whole environment, costs for its entire duration,
  and dies at 15 minutes. Buffered responses are capped at 6 MB; streamed at
  200 MB with 2 MBps after the first 6 MB. [L5][L7]
- Behind API Gateway the cap is 29–30 seconds per request instead. [A1][A2]

**Verdict.** MCP on Lambda means stateless sessions, JSON (not SSE) responses,
and no GET stream. That is a legitimate MCP server. It is also a narrower one
than the same binary serves on EC2, where the SDK's stateful default, SSE and
resumability all work unchanged.

## 5. Simplest-that-works cost and setup

Prices: US East (N. Virginia), on-demand, 730 hours/month. Free-tier credits for
new accounts ($100 at sign-up, up to $200, account "closes on its own 6 months
after you open it or when your credits run out" on the Free plan) are ignored;
the 12-month EC2 free tier for older accounts is not confirmed on the current
page. [E6]

### EC2

| Item | Price | Monthly |
| --- | --- | --- |
| `t4g.micro` (2 vCPU, 1 GiB, arm64) [E1] | $0.0084/h | $6.13 |
| or `t4g.nano` (2 vCPU, 0.5 GiB) [E1] | $0.0042/h | $3.07 |
| Public IPv4 [E2] | $0.005/h | $3.65 |
| EBS gp3 8 GB root [E4] | $0.08/GB-month (page example, region not named) | $0.64 |
| Route 53 hosted zone (optional) [R1] | $0.50/zone | $0.50 |
| TLS via Caddy + Let's Encrypt [T1] | — | $0 |
| **Total** | | **about $11 (micro) / $8 (nano)** |

Setup: one instance, one security group (22, 80, 443), Caddy with one line of
config reverse-proxying to `ubiq serve`, a systemd unit, `git clone` once, push
credentials in a file. Everything the map's "Not yet specified" list asks about
(sync, concurrent writes, cookie sessions, restarts) has ordinary single-process
answers.

### Lambda, function URL, clone in `/tmp`

| Item | Price | Monthly at low traffic |
| --- | --- | --- |
| Requests [L13] | $0.20/million, first 1M free | $0 |
| Duration x86 [L13] | $0.0000166667/GB-s, first 400,000 GB-s free | $0 |
| Duration arm64 [L13] | $0.0000133334/GB-s | $0 |
| Ephemeral storage above 512 MB [L13] | $0.0000000309/GB-s | ~$0 |
| Streaming beyond 6 MB/response [L13] | $0.008/GB, 100 GB free | $0 |
| Function URL [L9] | no charge | $0 |
| CloudFront for a custom name (optional) [C2] | Free plan $0/distribution | $0 |
| **Total** | | **about $0, plus any domain** |

Setup: container image (Go binary + git) or zip (Go binary + go-git), function
with 1–2 GB memory, function URL in `RESPONSE_STREAM` or buffered mode, reserved
concurrency 1 to serialise writes, MCP handler in `Stateless` + `JSONResponse`
mode, clone-on-first-use and pull-before-write logic, push credentials in
environment variables or Secrets Manager. OIDC redirect URI against the
generated hostname.

### Lambda with EFS (the version that keeps the clone)

Adds: VPC with two private subnets, EFS file system with two mount targets and
an access point, security groups for NFS, a NAT gateway (about $0.045/h, about
$33/month, Ohio price) for outbound traffic, and loses function URL streaming.
Not simpler or cheaper than EC2 in any respect. [L3][L4][L5][V2]

## Comparison table

| | EC2 `t4g.micro` + Caddy | Lambda + function URL, clone in `/tmp` | Lambda + EFS |
| --- | --- | --- | --- |
| Clone persists | Yes, local disk | No; per environment, re-fetched on cold start [L2] | Yes, shared NFS [L3] |
| Concurrent writers | One process, one lock | One per environment; reserved concurrency 1 to serialise [L6] | Shared disk, need file locking across environments |
| Needs `git` binary shipped | Package manager | Bundle in image or use go-git [L12][G1] | Same |
| Public HTTPS | Let's Encrypt via Caddy, own name [T1] | Built in, random stable hostname; CloudFront for own name [L8][C1] | Same, but streaming off in VPC [L5] |
| OIDC redirect URI | `https://your-name/...` | `https://<url-id>.lambda-url...on.aws/...` | Same |
| Max response duration | Unbounded | 900 s [L7] | 900 s |
| MCP sessions | Stateful, SSE, resumable | Stateless, JSON responses, GET 405 [M2] | Same as `/tmp` variant |
| Outbound internet | Direct | Direct | NAT gateway required [L4] |
| Monthly cost (low traffic) | ~$11 | ~$0 | ~$33+ (NAT) |
| Moving parts | 1 instance, 1 process | function, URL, image/layer, clone logic | + VPC, subnets, EFS, access point, NAT |

## Not verified

- **Git clone timing on Lambda.** No AWS source quantifies a clone of a few
  hundred files from a cold environment; only the generic sub-second cold start,
  the 10-second `Init` limit and the 625 Mbps bandwidth figure are documented.
- **Whether `provided.al2023` includes a `git` binary.** The runtime is described
  as minimal (<40 MB); the package list is not in the Lambda docs fetched.
- **Lambda Web Adapter and Server-Sent Events.** The README states "Supports
  response streaming" and has Go examples but says nothing specific about SSE.
- **Function URL maximum request duration.** Inferred from the 900-second
  function timeout and function URLs being synchronous invocations; no sentence
  on the function URL pages states it directly.
- **Lambda free tier as "always free".** The pricing page states 1M requests and
  400,000 GB-seconds per month without an expiry; the Free Tier pages describe
  a new 6-month credit model and do not list Lambda by name.
- **12-month EC2 free tier (750 h of `t2.micro`/`t3.micro`) for older accounts.**
  Not present on the current Free Tier pages.
- **EBS gp3 N. Virginia rate.** The page gives $0.08/GB-month only as an
  unnamed-region example. NAT gateway rate is quoted for US East (Ohio).
- **go-git compatibility limits** (shallow clone, performance). README points to
  a COMPATIBILITY.md not read here.

## Sources

AWS Lambda

- [L1] Ephemeral storage: https://docs.aws.amazon.com/lambda/latest/dg/configuration-ephemeral-storage.html
- [L2] Execution environment lifecycle, cold starts, `/tmp` persistence, Init limit: https://docs.aws.amazon.com/lambda/latest/dg/lambda-runtime-environment.html
- [L3] EFS with Lambda: https://docs.aws.amazon.com/lambda/latest/dg/configuration-filesystem-efs.html and https://docs.aws.amazon.com/lambda/latest/dg/configuration-filesystem.html
- [L4] VPC attachment and internet access: https://docs.aws.amazon.com/lambda/latest/dg/configuration-vpc.html
- [L5] Response streaming, bandwidth, VPC limitation, billing while streaming: https://docs.aws.amazon.com/lambda/latest/dg/configuration-response-streaming.html
- [L6] Concurrency and scaling, reserved concurrency: https://docs.aws.amazon.com/lambda/latest/dg/lambda-concurrency.html
- [L7] Quotas (timeout, memory, payload, bandwidth, image size, `/tmp`): https://docs.aws.amazon.com/lambda/latest/dg/gettingstarted-limits.html
- [L8] Function URLs, format, auth, throttling, deletion: https://docs.aws.amazon.com/lambda/latest/dg/urls-configuration.html and https://docs.aws.amazon.com/lambda/latest/dg/urls-invocation.html
- [L9] Function URL vs API Gateway (custom domains, cost): https://docs.aws.amazon.com/lambda/latest/dg/furls-http-invoke-decision.html
- [L10] Lambda Web Functions (Node.js only): https://docs.aws.amazon.com/lambda/latest/dg/web-functions.html
- [L11] Lambda Managed Instances: https://docs.aws.amazon.com/lambda/latest/dg/lambda-managed-instances.html
- [L12] Go on Lambda and OS-only runtimes: https://docs.aws.amazon.com/lambda/latest/dg/lambda-golang.html , https://docs.aws.amazon.com/lambda/latest/dg/runtimes-provided.html , https://aws.amazon.com/blogs/compute/introducing-the-amazon-linux-2023-runtime-for-aws-lambda/
- [L13] Lambda pricing page and Price List API offer file (us-east-1, published 2026-11-11): https://aws.amazon.com/lambda/pricing/ , https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AWSLambda/current/us-east-1/index.json
- [L14] Custom runtime response streaming (Runtime API): https://docs.aws.amazon.com/lambda/latest/dg/runtimes-custom.html

API Gateway and CloudFront

- [A1] HTTP API quotas: https://docs.aws.amazon.com/apigateway/latest/developerguide/http-api-quotas.html
- [A2] REST API quotas: https://docs.aws.amazon.com/apigateway/latest/developerguide/api-gateway-execution-service-limits-table.html
- [A3] REST API Lambda response streaming: https://docs.aws.amazon.com/apigateway/latest/developerguide/response-transfer-mode-lambda.html
- [A4] API Gateway pricing: https://aws.amazon.com/api-gateway/pricing/
- [C1] CloudFront with a Lambda function URL origin (OAC, signed POST bodies): https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-restricting-access-to-lambda.html
- [C2] CloudFront pricing (plans): https://aws.amazon.com/cloudfront/pricing/

EC2, networking, storage

- [E1] EC2 on-demand prices, AWS pricing data feed for the on-demand page, Linux, US East (N. Virginia): https://b0.p.awsstatic.com/pricing/2.0/meteredUnitMaps/ec2/USD/current/ec2-ondemand-without-sec-sel/US%20East%20(N.%20Virginia)/Linux/index.json (t4g.nano $0.0042, t4g.micro $0.0084, t4g.small $0.0168, t3.micro $0.0104, t3.small $0.0208, t3a.micro $0.0094 per hour); landing page https://aws.amazon.com/ec2/pricing/on-demand/
- [E2] Public IPv4 pricing: https://aws.amazon.com/vpc/pricing/
- [E3] ACM pricing: https://aws.amazon.com/certificate-manager/pricing/
- [E4] EBS pricing: https://aws.amazon.com/ebs/pricing/
- [E5] Elastic Load Balancing pricing: https://aws.amazon.com/elasticloadbalancing/pricing/
- [E6] AWS Free Tier: https://aws.amazon.com/free/ and https://aws.amazon.com/free/free-tier-faqs/
- [V2] NAT gateway pricing (worked example, US East (Ohio)): https://aws.amazon.com/vpc/pricing/
- [F1] EFS performance: https://docs.aws.amazon.com/efs/latest/ug/performance.html
- [F2] EFS pricing, Price List API offer file (us-east-1, published 2026-11-04): https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEFS/current/us-east-1/index.json ; landing page https://aws.amazon.com/efs/pricing/
- [R1] Route 53 pricing: https://aws.amazon.com/route53/pricing/
- [T1] Caddy automatic HTTPS: https://caddyserver.com/docs/automatic-https

Go libraries

- [G1] go-git: https://github.com/go-git/go-git
- [G2] aws-lambda-go: https://github.com/aws/aws-lambda-go
- [G3] `lambdaurl` package: https://pkg.go.dev/github.com/aws/aws-lambda-go/lambdaurl
- [G4] AWS Lambda Web Adapter: https://github.com/awslabs/aws-lambda-web-adapter

MCP

- [M1] MCP specification 2025-06-18, Transports (Streamable HTTP, sessions): https://modelcontextprotocol.io/specification/2025-06-18/basic/transports
- [M2] Go SDK `StreamableHTTPOptions` (v1.8.0 source): https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/mcp/streamable.go

OpenID Connect

- [O1] OpenID Connect Core 1.0, section 3.1.2.1 `redirect_uri`: https://openid.net/specs/openid-connect-core-1_0.html
