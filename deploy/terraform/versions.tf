terraform {
  required_version = ">= 1.5"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }

  # State is local (terraform.tfstate here) and gitignored; it holds the secrets from the variables.
}

provider "aws" {
  region = var.region
}
