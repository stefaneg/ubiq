# The Server runs beside its proxy on one instance

A Deployment is one small EC2 instance running Pomerium and the Server as two systemd units, Pomerium holding ports
80 and 443 with its own certificates and the Server listening on loopback only, under one wildcard DNS record so the
proxy's authenticate service, the page and the MCP route each get a hostname. Lambda was rejected: its disk is a
per-environment cache that would re-clone on every cold start, keeping the clone on EFS forces a VPC and a NAT gateway
that cost more than the instance, MCP would have to run stateless, and the proxy needs a long-running host regardless.
The Server has no notion of its public URL; that is the proxy's.

## Consequences

- In development the two run under Docker Compose, and the Server may also run alone with a flag that names a fixed
  Person; that flag works only on a loopback address, and production has no such mode.
- How the first instance is provisioned, by hand, by script, or by a baked image, is decided when the Deployment
  first goes to AWS, after it runs locally. Infrastructure as code belongs to production readiness.
