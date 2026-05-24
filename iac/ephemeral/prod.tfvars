aws_profile = "secwager-prod"

use_spot            = false
node_instance_types = ["t3.medium"]
node_min_size       = 2
node_max_size       = 5
node_desired_size   = 2

aurora_min_capacity        = 1.0
aurora_max_capacity        = 8.0
aurora_instance_count      = 1
aurora_skip_final_snapshot = false
aurora_deletion_protection = true

msk_broker_count  = 2
msk_instance_type = "kafka.t3.small"
