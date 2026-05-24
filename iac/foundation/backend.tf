terraform {
  backend "s3" {
    bucket         = "secwager-tfstate"
    key            = "foundation/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "secwager-tf-locks"
    encrypt        = true
  }
}
