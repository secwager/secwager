variable "name_prefix" {
  type = string
}

variable "repositories" {
  type    = list(string)
  default = ["cashier", "registry", "userregistration", "market", "btcwatcher"]
}

variable "environment" {
  type = string
}
