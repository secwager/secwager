output "eks_cluster_name" {
  value = module.eks.cluster_name
}

output "eks_cluster_endpoint" {
  value = module.eks.cluster_endpoint
}

output "aurora_endpoint" {
  value = module.aurora.endpoint
}

output "msk_bootstrap_brokers" {
  value = module.msk.bootstrap_brokers
}

output "userregistration_irsa_role_arn" {
  value = aws_iam_role.userregistration.arn
}

# Retrieve DSNs with: terraform output -raw cashier_dsn
output "cashier_dsn" {
  value     = module.aurora.cashier_dsn
  sensitive = true
}

output "registry_dsn" {
  value     = module.aurora.registry_dsn
  sensitive = true
}

output "userregistration_dsn" {
  value     = module.aurora.userregistration_dsn
  sensitive = true
}

# kubeconfig update command
output "kubeconfig_cmd" {
  value = "aws eks update-kubeconfig --region ${var.region} --name ${module.eks.cluster_name}"
}
