output "kubernetes_cluster_id" {
  description = "Managed Kubernetes cluster ID"
  value       = module.kubernetes.cluster_id
}

output "kubernetes_kubeconfig" {
  description = "Kubeconfig for cluster access"
  value       = module.kubernetes.kubeconfig
  sensitive   = true
}

output "postgresql_host" {
  description = "PostgreSQL cluster host (FQDN)"
  value       = module.postgresql.host
}

output "postgresql_port" {
  value = 6432
}

output "object_storage_bucket" {
  description = "Main documents S3 bucket name"
  value       = module.storage.documents_bucket
}

output "vpc_id" {
  value = module.vpc.network_id
}
