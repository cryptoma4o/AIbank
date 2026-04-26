variable "project"     { type = string }
variable "environment" { type = string }
variable "network_id"  { type = string }
variable "subnet_ids"  { type = map(string) }
variable "pg_version"  { type = string; default = "16" }
variable "disk_size"   { type = number; default = 100 }
variable "db_password" { type = string; sensitive = true }
