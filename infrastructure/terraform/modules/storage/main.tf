resource "yandex_iam_service_account" "storage" {
  name = "${var.project}-${var.environment}-storage-sa"
}

resource "yandex_resourcemanager_folder_iam_member" "storage_admin" {
  folder_id = var.folder_id
  role      = "storage.admin"
  member    = "serviceAccount:${yandex_iam_service_account.storage.id}"
}

resource "yandex_iam_service_account_static_access_key" "storage" {
  service_account_id = yandex_iam_service_account.storage.id
  description        = "Static key for Object Storage access"
}

resource "yandex_storage_bucket" "documents" {
  bucket     = "${var.project}-${var.environment}-documents"
  acl        = "private"
  access_key = yandex_iam_service_account_static_access_key.storage.access_key
  secret_key = yandex_iam_service_account_static_access_key.storage.secret_key

  versioning {
    enabled = true
  }

  server_side_encryption_configuration {
    rule {
      apply_server_side_encryption_by_default {
        sse_algorithm = "aws:kms"
      }
    }
  }

  lifecycle_rule {
    enabled = true
    noncurrent_version_expiration {
      days = 90
    }
  }
}

resource "yandex_storage_bucket" "terraform_state" {
  bucket     = "${var.project}-terraform-state"
  acl        = "private"
  access_key = yandex_iam_service_account_static_access_key.storage.access_key
  secret_key = yandex_iam_service_account_static_access_key.storage.secret_key
}
