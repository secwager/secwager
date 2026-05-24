variable "name" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "subnet_ids" {
  type        = list(string)
  description = "Must span at least 2 AZs"
}

variable "allowed_security_group_ids" {
  type        = list(string)
  description = "Security groups allowed to reach port 5432 (e.g. EKS cluster SG)"
}

variable "master_username" {
  type    = string
  default = "secwager"
}

variable "min_capacity" {
  type    = number
  default = 0.5
}

variable "max_capacity" {
  type    = number
  default = 4.0
}

variable "instance_count" {
  type    = number
  default = 1
}

variable "skip_final_snapshot" {
  type    = bool
  default = true
}

variable "deletion_protection" {
  type    = bool
  default = false
}
