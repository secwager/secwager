output "endpoint" {
  value = aws_rds_cluster.this.endpoint
}

output "reader_endpoint" {
  value = aws_rds_cluster.this.reader_endpoint
}

output "master_username" {
  value = aws_rds_cluster.this.master_username
}

output "master_password" {
  value     = random_password.master.result
  sensitive = true
}

output "security_group_id" {
  value = aws_security_group.this.id
}

# Convenience DSN outputs — retrieve with: terraform output -raw cashier_dsn
output "cashier_dsn" {
  value     = "host=${aws_rds_cluster.this.endpoint} port=5432 dbname=cashier user=${aws_rds_cluster.this.master_username} password=${random_password.master.result} sslmode=require"
  sensitive = true
}

output "registry_dsn" {
  value     = "host=${aws_rds_cluster.this.endpoint} port=5432 dbname=registry user=${aws_rds_cluster.this.master_username} password=${random_password.master.result} sslmode=require"
  sensitive = true
}

output "userregistration_dsn" {
  value     = "host=${aws_rds_cluster.this.endpoint} port=5432 dbname=userregistration user=${aws_rds_cluster.this.master_username} password=${random_password.master.result} sslmode=require"
  sensitive = true
}
