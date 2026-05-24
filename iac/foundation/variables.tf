variable "region" {
  type    = string
  default = "us-east-1"
}

variable "aws_profile" {
  type = string
}

variable "single_nat_gateway" {
  type = bool
}
