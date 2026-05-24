terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.50"
    }
  }
}

resource "aws_security_group" "this" {
  name_prefix = "${var.name}-msk-"
  vpc_id      = var.vpc_id
  description = "MSK Kafka"

  ingress {
    description     = "Kafka plaintext"
    from_port       = 9092
    to_port         = 9092
    protocol        = "tcp"
    security_groups = var.allowed_security_group_ids
  }

  ingress {
    description     = "Kafka TLS"
    from_port       = 9094
    to_port         = 9094
    protocol        = "tcp"
    security_groups = var.allowed_security_group_ids
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.name}-msk" }
}

resource "aws_msk_cluster" "this" {
  cluster_name           = var.name
  kafka_version          = var.kafka_version
  number_of_broker_nodes = var.broker_count

  broker_node_group_info {
    instance_type  = var.instance_type
    client_subnets = var.subnet_ids
    security_groups = [aws_security_group.this.id]

    storage_info {
      ebs_storage_info {
        volume_size = var.ebs_volume_size
      }
    }
  }

  encryption_info {
    encryption_in_transit {
      # TLS_PLAINTEXT allows both; market service connects plaintext within VPC
      client_broker = "TLS_PLAINTEXT"
      in_cluster    = true
    }
  }

  configuration_info {
    arn      = aws_msk_configuration.this.arn
    revision = aws_msk_configuration.this.latest_revision
  }
}

resource "aws_msk_configuration" "this" {
  name              = var.name
  kafka_versions    = [var.kafka_version]

  server_properties = <<-EOT
    auto.create.topics.enable=true
    default.replication.factor=1
    min.insync.replicas=1
    num.partitions=4
    log.retention.hours=168
  EOT
}
