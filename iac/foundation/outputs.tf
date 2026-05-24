output "vpc_id" {
  value = module.vpc.vpc_id
}

output "public_subnet_ids" {
  value = module.vpc.public_subnet_ids
}

output "private_subnet_ids" {
  value = module.vpc.private_subnet_ids
}

output "cognito_user_pool_id" {
  value = module.cognito.user_pool_id
}

output "cognito_client_id" {
  value = module.cognito.client_id
}

output "kms_key_arn" {
  value = module.kms.key_arn
}

output "kms_key_id" {
  value = module.kms.key_id
}

output "eks_cluster_role_arn" {
  value = module.iam.eks_cluster_role_arn
}

output "eks_node_role_arn" {
  value = module.iam.eks_node_role_arn
}

output "ecr_repository_urls" {
  value = module.ecr.repository_urls
}
