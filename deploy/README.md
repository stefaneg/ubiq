# Deploying the Server to AWS

One Deployment is one `t4g.micro` running Pomerium and `ubiq serve` as two systemd units. Packer bakes the image,
Terraform places the instance, both from a Maintainer's machine; there is no pipeline. The design, and why each piece
is there, is [docs/server.md](../docs/server.md), "What the Deployment owns" and "Going to AWS"; this is how to run it.
Nothing here is tested in Go: the smoke test below is the acceptance.

| Path | What |
|---|---|
| `packer/ubiq.pkr.hcl` | The image: Amazon Linux 2023 arm64, Pomerium from its rpm repository, git, the ubiq binary. |
| `packer/install.sh` | Runs on Packer's build instance; installs everything, enables nothing. |
| `packer/files/ubiq.service` | The Server's unit. Its `ExecStart` line is the whole configuration, filled in at first boot. |
| `packer/files/pomerium-ubiq.conf` | Drop-in for Pomerium's own unit: bind 80 and 443, state under `/var/lib/pomerium`. |
| `packer/files/pomerium.yaml.tmpl` | Pomerium's configuration, the reference file for any Deployment. |
| `packer/files/first-boot.sh` | Renders both from `/etc/ubiq/provision.env`, generates Pomerium's secrets, writes the push credential, starts both units. |
| `terraform/` | The instance, its Elastic IP, a security group with 80 and 443 in, an SSM instance role, the optional Route53 record, the user-data. |
| `terraform/terraform.tfvars.example` | The variables, filled in for the test Deployment. |

Terraform state is local, in `terraform/`, and gitignored with `terraform.tfvars`: both hold the secrets. Keep them
on the Maintainer's machine.

## What you need

An AWS account and credentials for it in your shell, Go, [Packer](https://developer.hashicorp.com/packer/install),
[Terraform](https://developer.hashicorp.com/terraform/install) 1.5 or later, and for logging in to the instance the
AWS CLI with the [Session Manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html).
The instance goes in the region's default VPC.

## By hand, in this order

1. **The variables.** `cp terraform/terraform.tfvars.example terraform/terraform.tfvars` and pick the region, and
   the domain if there is one. With no domain the hostnames hang off the Elastic IP, so place it first and read the
   callback URL the Identity Provider needs:

   ```sh
   cd deploy/terraform
   terraform init
   terraform apply -target=aws_eip.ubiq   # only when domain is empty
   terraform output callback_url          # https://authenticate.<domain>/oauth2/callback
   ```

   With a domain the callback URL is `https://authenticate.<domain>/oauth2/callback`.

2. **The Identity Provider's OAuth client**, a confidential web client whose redirect URI is the callback URL. For
   Google: Cloud console, APIs & Services, Credentials, OAuth client ID of type Web application; the issuer is
   `https://accounts.google.com`, and since Google sends no `groups` the Writer Rule is `writer_claim = "email"` with
   `writers` the Writers' addresses. Put the client id and secret and the Writer Rule in `terraform.tfvars`.

3. **The push credential**, scoped to the one repository, with the longest lifetime the host allows and a calendar
   reminder for its expiry:
   - GitHub: a dedicated machine account, a collaborator with write on the repository, and a fine-grained personal
     access token on it for that repository with contents read and write. `push_account` is the account,
     `committer` its name and email.
   - GitLab: a project access token with role Developer or above and scope `write_repository`. GitLab creates its bot
     user; `push_account` is the bot's username, `committer` its name and email.

   Put `origin` (the repository's https URL), `push_account`, `push_token` and `committer` in `terraform.tfvars`.

4. **Keep merged Batches.** The Server tells a merged Batch from one a Maintainer opened by hand only by the earlier
   `ubiq/<n>` branches on origin, so Maintainers keep every merged `ubiq/*` branch. On GitHub, turn off
   "Automatically delete head branches" in the repository's settings, or protect `ubiq/*` from deletion with a
   ruleset; on GitLab, turn off "Enable 'Delete source branch' option by default" and leave the option unticked on
   each merge request from a Batch. When merged Batches are deleted anyway, a Batch opened by hand afterwards is taken
   as merged and skipped, so the next Change opens the Batch numbered after it; and once no `ubiq/*` branch is left,
   numbering starts again at `ubiq/1`, a name the forge may still show on the first Batch's old pull request.

5. **Bake**, from the repository root:

   ```sh
   GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o deploy/build/ubiq .
   cd deploy/packer
   packer init .
   packer build -var region=<region> .
   ```

   The image is named `ubiq-<timestamp>`; Terraform takes the newest one unless `ami` is set.

6. **Apply**: `cd deploy/terraform && terraform apply`. The outputs are the page, the MCP endpoint and the instance.

7. **DNS**, only when a domain is used without `route53_zone_id`: a wildcard `A` record `*.<domain>` to the
   `public_ip` output. With a zone, Terraform writes it; with no domain, sslip.io answers by itself.

First boot takes a minute or two after the instance is up, then Pomerium fetches Let's Encrypt certificates for
`authenticate.`, `ubiq.` and `mcp.` as it starts; until they arrive, browsers see a certificate error and the Server
cannot fetch the assertion keys. Let's Encrypt's rate limits apply per registered
domain; on sslip.io they are shared with everyone else using it.

## The smoke test

A Deployment works when, once against a GitHub origin and once against a GitLab one:

1. **Log in.** Open the `page` output in a browser, log in at the Identity Provider, and see the page with your name
   and role at the top: Writer when the Writer Rule admits you.
2. **Connect Claude Code.** `claude mcp add --transport http ubiq <mcp output>`, then `/mcp` in Claude Code and
   authenticate; the same login opens in the browser.
3. **`whoami`.** Ask the Agent to call it: your name, email and role Writer.
4. **`define_term`.** Ask the Agent to define a throwaway Term. The result names the commit and the Batch, and a
   `lookup` of the Term finds it.
5. **See the commit on the Batch.** On the forge, the branch `ubiq/<n>` the result named holds one commit
   `Define Term <name>`, authored by you and committed by `committer`. Delete the branch afterwards, or remove the
   Term with `remove_term` and let the Batch be merged.

When something fails, open a shell with `aws ssm start-session --target <instance_id>` and read
`/var/log/cloud-init-output.log` (first boot), `journalctl -u pomerium` and `journalctl -u ubiq` (one line per request
and per Change).

## Upgrading and rotating are one act: replace the instance

A new ubiq binary, a new Pomerium, a new push token, a new Identity Provider secret, a changed Writer Rule: change the
input and replace the instance.

1. If the binary or the image changed: build and `packer build` as in Bake above.
2. Change `terraform.tfvars` if a variable changed.
3. `terraform apply`. A new image or new user-data replaces the instance; the Elastic IP, and so the hostnames, stay.

The Server re-clones origin on its first start, and every Batch lives on origin, so nothing is lost but Pomerium's
sessions: everyone logs in again, Agents included. `terraform apply -replace=aws_instance.ubiq` replaces it with no
input changed.

Secrets ride in user-data and in the local state, readable by anyone with EC2 describe rights on the account; that is
accepted for this scope, and SSM Parameter Store is the hardening step after it.

## Out of scope

Production readiness: no pipeline runs Packer or Terraform, no monitoring, no backups (origin is the only state worth
keeping), no high availability, no rate limits. `terraform destroy` removes everything but the images; deregister
those in the EC2 console.
