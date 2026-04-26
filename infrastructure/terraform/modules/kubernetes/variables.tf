variable "project"            { type = string }
variable "environment"        { type = string }
variable "folder_id"          { type = string }
variable "network_id"         { type = string }
variable "private_subnet_ids" { type = map(string) }
variable "k8s_sg_id"          { type = string }
variable "node_platform"      { type = string; default = "standard-v3" }
variable "node_cores"         { type = number; default = 4 }
variable "node_memory"        { type = number; default = 8 }
variable "node_count"         { type = number; default = 2 }
variable "kms_key_id"         { type = string; default = "" }
