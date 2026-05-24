terraform {
  required_version = ">= 1.6.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.50"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

provider "aws" {
  region  = var.region
  profile = var.aws_profile

  default_tags {
    tags = {
      Project   = "secwager"
      ManagedBy = "terraform"
      Workspace = terraform.workspace
    }
  }
}

locals {
  name = "secwager-${terraform.workspace}"
  env  = terraform.workspace
}

module "eks" {
  source = "../modules/eks"

  cluster_name       = local.name
  kubernetes_version = var.kubernetes_version
  cluster_role_arn   = local.f.eks_cluster_role_arn
  node_role_arn      = local.f.eks_node_role_arn
  vpc_id             = local.f.vpc_id
  subnet_ids         = local.f.private_subnet_ids

  node_instance_types = var.node_instance_types
  use_spot            = var.use_spot
  node_min_size       = var.node_min_size
  node_max_size       = var.node_max_size
  node_desired_size   = var.node_desired_size
}

module "aurora" {
  source = "../modules/aurora"

  name       = local.name
  vpc_id     = local.f.vpc_id
  subnet_ids = local.f.private_subnet_ids

  allowed_security_group_ids = [module.eks.cluster_security_group_id]

  min_capacity        = var.aurora_min_capacity
  max_capacity        = var.aurora_max_capacity
  instance_count      = var.aurora_instance_count
  skip_final_snapshot = var.aurora_skip_final_snapshot
  deletion_protection = var.aurora_deletion_protection
}

module "msk" {
  source = "../modules/msk"

  name       = local.name
  vpc_id     = local.f.vpc_id
  subnet_ids = slice(local.f.private_subnet_ids, 0, var.msk_broker_count)

  allowed_security_group_ids = [module.eks.cluster_security_group_id]

  broker_count  = var.msk_broker_count
  instance_type = var.msk_instance_type
}

# IRSA role for userregistration — grants KMS access; destroyed with cluster
data "aws_iam_policy_document" "userregistration_assume" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    effect  = "Allow"
    principals {
      type        = "Federated"
      identifiers = [module.eks.oidc_provider_arn]
    }
    condition {
      test     = "StringEquals"
      variable = "${module.eks.oidc_provider_url}:sub"
      values   = ["system:serviceaccount:default:userregistration"]
    }
    condition {
      test     = "StringEquals"
      variable = "${module.eks.oidc_provider_url}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "userregistration" {
  name               = "${local.name}-userregistration"
  assume_role_policy = data.aws_iam_policy_document.userregistration_assume.json
}

resource "aws_iam_role_policy" "userregistration_kms" {
  name = "kms-access"
  role = aws_iam_role.userregistration.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["kms:Encrypt", "kms:Decrypt", "kms:GenerateDataKey"]
      Resource = local.f.kms_key_arn
    }]
  })
}

# KMS grant lets the IRSA role use the key without modifying the key policy in foundation
resource "aws_kms_grant" "userregistration" {
  name              = "userregistration-${local.env}"
  key_id            = local.f.kms_key_id
  grantee_principal = aws_iam_role.userregistration.arn
  operations        = ["Encrypt", "Decrypt", "GenerateDataKey"]
}
