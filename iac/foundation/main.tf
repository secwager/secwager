terraform {
  required_version = ">= 1.6.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.50"
    }
  }
}

provider "aws" {
  region  = var.region
  profile = var.aws_profile

  default_tags {
    tags = {
      Project     = "secwager"
      ManagedBy   = "terraform"
      Workspace   = terraform.workspace
    }
  }
}

locals {
  name = "secwager-${terraform.workspace}"
  env  = terraform.workspace
}

module "vpc" {
  source = "../modules/vpc"

  name               = local.name
  single_nat_gateway = var.single_nat_gateway
}

module "cognito" {
  source = "../modules/cognito"

  name        = local.name
  environment = local.env
}

module "kms" {
  source = "../modules/kms"

  alias_name  = "secwager-userregistration-${local.env}"
  description = "secwager userregistration private key encryption (${local.env})"
  environment = local.env
}

module "iam" {
  source = "../modules/iam"

  name_prefix = local.name
}

module "ecr" {
  source = "../modules/ecr"

  name_prefix  = "secwager"
  environment  = local.env
}
