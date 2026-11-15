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
- The instance is produced by Packer and Terraform run from a Maintainer's machine, no pipeline: Packer bakes an
  Amazon Linux 2023 arm64 image holding Pomerium's package, the ubiq binary and both units; Terraform places one
  instance with an Elastic IP, opens ports 80 and 443 only, and hands it its settings as user-data. The instance is
  disposable: an upgrade or a credential rotation replaces it, and the Server re-clones on first start. Without a
  domain the hostnames come from sslip.io, since Let's Encrypt refuses Amazon's default names; Pomerium fetches its
  own certificates either way. High availability, monitoring and backups remain production readiness.
