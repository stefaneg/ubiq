# One Deployment: one instance from the image deploy/packer baked, an Elastic IP, ports 80 and 443 in and nothing
# else, SSM Session Manager instead of SSH, and user-data that lands as /etc/ubiq/provision.env for first boot.
# Changing any input replaces the instance: that is the upgrade and the credential rotation. See deploy/README.md.

data "aws_ami" "ubiq" {
  count       = var.ami == "" ? 1 : 0
  owners      = ["self"]
  most_recent = true

  filter {
    name   = "name"
    values = ["ubiq-*"]
  }

  filter {
    name   = "tag:ubiq"
    values = ["image"]
  }
}

data "aws_vpc" "default" {
  default = true
}

# The address exists before the instance, so the sslip.io hostnames can be rendered into its user-data.
resource "aws_eip" "ubiq" {
  domain = "vpc"
  tags   = { Name = "ubiq" }
}

locals {
  domain = var.domain != "" ? var.domain : "${replace(aws_eip.ubiq.public_ip, ".", "-")}.sslip.io"

  # /etc/ubiq/provision.env, sourced by first boot; every value single-quoted for the shell.
  provision = {
    DOMAIN            = local.domain
    IDP_ISSUER        = var.idp_issuer
    IDP_CLIENT_ID     = var.idp_client_id
    IDP_CLIENT_SECRET = var.idp_client_secret
    WRITER_CLAIM      = var.writer_claim
    WRITERS           = join(",", [for w in var.writers : trimspace(w)])
    ORIGIN            = var.origin
    PUSH_ACCOUNT      = var.push_account
    PUSH_TOKEN        = var.push_token
    COMMITTER         = var.committer
  }
  provision_env = join("", [for k, v in local.provision : "${k}='${replace(v, "'", "'\\''")}'\n"])

  user_data = "#cloud-config\n${yamlencode({
    write_files = [{
      path        = "/etc/ubiq/provision.env"
      owner       = "root:root"
      permissions = "0600"
      encoding    = "b64"
      content     = base64encode(local.provision_env)
    }]
    runcmd = [["/usr/local/libexec/ubiq/first-boot"]]
  })}"
}

resource "aws_security_group" "ubiq" {
  name        = "ubiq"
  description = "ubiq Deployment: Pomerium on 80 and 443, nothing else in"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "HTTP, redirect and Let's Encrypt HTTP-01"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "HTTPS"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    description = "origin, the Identity Provider, Let's Encrypt, SSM"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_iam_role" "ubiq" {
  name = "ubiq"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.ubiq.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "ubiq" {
  name = "ubiq"
  role = aws_iam_role.ubiq.name
}

resource "aws_instance" "ubiq" {
  ami                         = var.ami != "" ? var.ami : data.aws_ami.ubiq[0].id
  instance_type               = var.instance_type
  iam_instance_profile        = aws_iam_instance_profile.ubiq.name
  vpc_security_group_ids      = [aws_security_group.ubiq.id]
  user_data                   = local.user_data
  user_data_replace_on_change = true

  metadata_options {
    http_tokens = "required"
  }

  root_block_device {
    volume_type = "gp3"
    encrypted   = true
  }

  tags = { Name = "ubiq" }

  lifecycle {
    precondition {
      condition     = var.route53_zone_id == "" || var.domain != ""
      error_message = "route53_zone_id needs domain; with no domain the hostnames are sslip.io's."
    }
  }
}

resource "aws_eip_association" "ubiq" {
  allocation_id = aws_eip.ubiq.id
  instance_id   = aws_instance.ubiq.id
}

# authenticate., ubiq. and mcp. under the domain, all to the Elastic IP.
resource "aws_route53_record" "wildcard" {
  count   = var.domain != "" && var.route53_zone_id != "" ? 1 : 0
  zone_id = var.route53_zone_id
  name    = "*.${var.domain}"
  type    = "A"
  ttl     = 300
  records = [aws_eip.ubiq.public_ip]
}
