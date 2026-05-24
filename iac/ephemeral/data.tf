data "terraform_remote_state" "foundation" {
  backend = "s3"
  config = {
    bucket = "secwager-tfstate"
    key    = "foundation/terraform.tfstate"
    region = "us-east-1"
  }
  workspace = terraform.workspace
}

locals {
  f = data.terraform_remote_state.foundation.outputs
}
