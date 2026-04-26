output "documents_bucket"   { value = yandex_storage_bucket.documents.bucket }
output "storage_access_key" {
  value     = yandex_iam_service_account_static_access_key.storage.access_key
  sensitive = true
}
output "storage_secret_key" {
  value     = yandex_iam_service_account_static_access_key.storage.secret_key
  sensitive = true
}
