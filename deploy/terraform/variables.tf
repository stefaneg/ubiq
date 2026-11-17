variable "region" {
  type        = string
  description = "The AWS region; the image must have been baked there."
}

variable "ami" {
  type        = string
  default     = ""
  description = "The image to run; empty takes the newest ubiq-* image this account baked."
}

variable "instance_type" {
  type    = string
  default = "t4g.micro"
}

variable "domain" {
  type        = string
  default     = ""
  description = "The Deployment's domain, under which authenticate., ubiq. and mcp. are its hostnames. Empty derives <eip-with-dashes>.sslip.io."

  validation {
    condition     = can(regex("^([a-z0-9-]+\\.)*[a-z0-9-]*$", var.domain))
    error_message = "domain is a lower-case DNS name, or empty for sslip.io."
  }
}

variable "route53_zone_id" {
  type        = string
  default     = ""
  description = "A Route53 hosted zone for domain; when given, Terraform writes the wildcard record. Otherwise the record is a by-hand step."
}

variable "idp_issuer" {
  type        = string
  description = "The Identity Provider's OpenID Connect issuer, e.g. https://accounts.google.com."
}

variable "idp_client_id" {
  type = string
}

variable "idp_client_secret" {
  type      = string
  sensitive = true
}

variable "writer_claim" {
  type        = string
  default     = "groups"
  description = "The Writer Rule's claim; email for Google, which sends no groups."
}

variable "writers" {
  type        = list(string)
  description = "The Writer Rule's accepted values."

  validation {
    condition     = length(var.writers) > 0 && alltrue([for w in var.writers : trimspace(w) != "" && !strcontains(w, ",")])
    error_message = "writers needs at least one value, none empty and none holding a comma."
  }
}

variable "origin" {
  type        = string
  description = "The repository's https URL."

  validation {
    condition     = can(regex("^https://[^/@]+/", var.origin))
    error_message = "origin is the repository's https URL, without credentials; the push token is push_token."
  }
}

variable "push_account" {
  type        = string
  description = "The account the push token belongs to: a GitHub machine account, or a GitLab project access token's bot user."

  validation {
    condition     = can(regex("^[A-Za-z0-9._~-]+$", var.push_account))
    error_message = "push_account must be a plain user name; it is written into a credential URL unescaped."
  }
}

variable "push_token" {
  type        = string
  sensitive   = true
  description = "A token scoped to the one repository: GitHub fine-grained with contents read and write, or GitLab with write_repository."

  validation {
    condition     = can(regex("^[A-Za-z0-9._~-]+$", var.push_token))
    error_message = "push_token must be URL-safe; it is written into a credential URL unescaped."
  }
}

variable "committer" {
  type        = string
  description = "The Server's git identity, \"Name <email>\", naming the push account."

  validation {
    condition     = can(regex("^[^<>]+ <[^<>@ ]+@[^<> ]+>$", var.committer))
    error_message = "committer is \"Name <email>\"."
  }
}
