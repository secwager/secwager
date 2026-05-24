output "cluster_name" {
  value = aws_eks_cluster.this.name
}

output "cluster_endpoint" {
  value = aws_eks_cluster.this.endpoint
}

output "cluster_ca_certificate" {
  value = aws_eks_cluster.this.certificate_authority[0].data
}

# Used by ephemeral layer to build IRSA role trust policies
output "oidc_provider_arn" {
  value = aws_iam_openid_connect_provider.eks.arn
}

# URL without https:// prefix — used in trust policy condition keys
output "oidc_provider_url" {
  value = trimprefix(aws_eks_cluster.this.identity[0].oidc[0].issuer, "https://")
}

# Cluster-managed SG that all nodes belong to — used to scope Aurora/MSK ingress rules
output "cluster_security_group_id" {
  value = aws_eks_cluster.this.vpc_config[0].cluster_security_group_id
}
