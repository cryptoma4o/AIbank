output "cluster_id" { value = yandex_kubernetes_cluster.main.id }
output "kubeconfig" {
  value     = yandex_kubernetes_cluster.main.id
  # Real kubeconfig obtained via: yc managed-kubernetes cluster get-credentials
  sensitive = true
}
