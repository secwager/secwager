aws_profile = "secwager-dev"

use_spot            = true
node_instance_types = ["t3.medium"]
node_min_size       = 1
node_max_size       = 2
node_desired_size   = 1

aurora_min_capacity        = 0.5
aurora_max_capacity        = 2.0
aurora_instance_count      = 1
aurora_skip_final_snapshot = true
aurora_deletion_protection = false

msk_broker_count  = 1
msk_instance_type = "kafka.t3.small"
