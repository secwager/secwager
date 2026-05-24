variable "name" {
  type = string
}

variable "vpc_id" {
  type = string
}

# Must contain exactly broker_count subnets, one per broker, in distinct AZs
variable "subnet_ids" {
  type = list(string)
}

variable "allowed_security_group_ids" {
  type        = list(string)
  description = "Security groups allowed to reach Kafka ports (e.g. EKS cluster SG)"
}

variable "kafka_version" {
  type    = string
  default = "3.7.x"
}

variable "broker_count" {
  type    = number
  default = 1
}

variable "instance_type" {
  type    = string
  default = "kafka.t3.small"
}

variable "ebs_volume_size" {
  type    = number
  default = 20
}
