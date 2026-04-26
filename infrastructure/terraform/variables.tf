variable "yc_token" {
  description = "Yandex Cloud IAM token or service account key"
  type        = string
  sensitive   = true
}

variable "yc_cloud_id" {
  description = "Yandex Cloud cloud ID"
  type        = string
}

variable "yc_folder_id" {
  description = "Yandex Cloud folder ID"
  type        = string
}

variable "yc_zone" {
  description = "Default availability zone"
  type        = string
  default     = "ru-central1-a"
}

variable "environment" {
  description = "Deployment environment: staging | prod"
  type        = string
  default     = "staging"
}

variable "project" {
  description = "Project name prefix for resource naming"
  type        = string
  default     = "aibank"
}

variable "k8s_node_count" {
  description = "Number of Kubernetes worker nodes per zone"
  type        = number
  default     = 2
}

variable "k8s_node_platform" {
  description = "Yandex Cloud platform ID for K8s nodes"
  type        = string
  default     = "standard-v3"
}

variable "k8s_node_cores" {
  type    = number
  default = 4
}

variable "k8s_node_memory" {
  description = "RAM in GB"
  type        = number
  default     = 8
}

variable "pg_version" {
  type    = string
  default = "16"
}

variable "pg_disk_size" {
  description = "PostgreSQL disk size in GB"
  type        = number
  default     = 100
}

variable "db_password" {
  description = "PostgreSQL aibank user password"
  type        = string
  sensitive   = true
}
