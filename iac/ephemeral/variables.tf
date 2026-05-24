variable "region" {
  type    = string
  default = "us-east-1"
}

variable "aws_profile" {
  type = string
}

variable "kubernetes_version" {
  type    = string
  default = "1.31"
}

variable "use_spot" {
  type = bool
}

variable "node_instance_types" {
  type    = list(string)
  default = ["t3.medium"]
}

variable "node_min_size" {
  type = number
}

variable "node_max_size" {
  type = number
}

variable "node_desired_size" {
  type = number
}

variable "aurora_min_capacity" {
  type = number
}

variable "aurora_max_capacity" {
  type = number
}

variable "aurora_instance_count" {
  type    = number
  default = 1
}

variable "aurora_skip_final_snapshot" {
  type = bool
}

variable "aurora_deletion_protection" {
  type = bool
}

variable "msk_broker_count" {
  type = number
}

variable "msk_instance_type" {
  type    = string
  default = "kafka.t3.small"
}
