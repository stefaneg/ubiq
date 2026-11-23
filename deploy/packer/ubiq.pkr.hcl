# The Deployment's image: Amazon Linux 2023 arm64 for a t4g.micro, with Pomerium from its rpm repository, the ubiq
# binary cross-compiled on the Maintainer's machine, both units, the Pomerium configuration template and the first-boot
# script. Nothing here holds a secret: those arrive as user-data from deploy/terraform. See deploy/README.md.

packer {
  required_plugins {
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = ">= 1.3.0"
    }
  }
}

variable "region" {
  type        = string
  description = "The AWS region to bake in; the same region deploy/terraform places the instance in."
}

variable "ubiq_binary" {
  type        = string
  default     = ""
  description = "The ubiq binary built with GOOS=linux GOARCH=arm64; deploy/build/ubiq when empty."
}

variable "pomerium_version" {
  type        = string
  default     = "0.34.0"
  description = "Pomerium's rpm version; the dev stack runs the same."
}

locals {
  stamp  = formatdate("YYYYMMDDhhmmss", timestamp())
  binary = var.ubiq_binary != "" ? var.ubiq_binary : "${path.root}/../build/ubiq"
}

source "amazon-ebs" "ubiq" {
  region        = var.region
  instance_type = "t4g.micro"
  ssh_username  = "ec2-user"
  ami_name      = "ubiq-${local.stamp}"

  source_ami_filter {
    filters = {
      name                = "al2023-ami-2023.*-arm64"
      architecture        = "arm64"
      virtualization-type = "hvm"
      root-device-type    = "ebs"
    }
    owners      = ["amazon"]
    most_recent = true
  }

  tags = {
    Name = "ubiq-${local.stamp}"
    ubiq = "image"
  }
}

build {
  sources = ["source.amazon-ebs.ubiq"]

  provisioner "shell" {
    inline = ["mkdir -p /tmp/ubiq-files"]
  }

  provisioner "file" {
    source      = local.binary
    destination = "/tmp/ubiq"
  }

  provisioner "file" {
    source      = "${path.root}/files/"
    destination = "/tmp/ubiq-files"
  }

  provisioner "shell" {
    script           = "${path.root}/install.sh"
    environment_vars = ["POMERIUM_VERSION=${var.pomerium_version}"]
    execute_command  = "chmod +x {{ .Path }}; sudo env {{ .Vars }} {{ .Path }}"
  }
}
