resource "yandex_iam_service_account" "k8s" {
  name = "${var.project}-${var.environment}-k8s-sa"
}

resource "yandex_resourcemanager_folder_iam_member" "k8s_editor" {
  folder_id = var.folder_id
  role      = "editor"
  member    = "serviceAccount:${yandex_iam_service_account.k8s.id}"
}

resource "yandex_resourcemanager_folder_iam_member" "k8s_images_puller" {
  folder_id = var.folder_id
  role      = "container-registry.images.puller"
  member    = "serviceAccount:${yandex_iam_service_account.k8s.id}"
}

resource "yandex_kubernetes_cluster" "main" {
  name       = "${var.project}-${var.environment}-k8s"
  network_id = var.network_id

  master {
    regional {
      region = "ru-central1"

      dynamic "location" {
        for_each = ["ru-central1-a", "ru-central1-b", "ru-central1-d"]
        content {
          zone      = location.value
          subnet_id = var.private_subnet_ids[location.value]
        }
      }
    }

    version   = "1.30"
    public_ip = false

    maintenance_policy {
      auto_upgrade = true
      maintenance_window {
        day        = "sunday"
        start_time = "02:00"
        duration   = "3h"
      }
    }
  }

  service_account_id      = yandex_iam_service_account.k8s.id
  node_service_account_id = yandex_iam_service_account.k8s.id

  release_channel = "STABLE"

  kms_provider {
    key_id = var.kms_key_id
  }
}

resource "yandex_kubernetes_node_group" "workers" {
  cluster_id = yandex_kubernetes_cluster.main.id
  name       = "${var.project}-${var.environment}-workers"
  version    = "1.30"

  instance_template {
    platform_id = var.node_platform

    resources {
      cores         = var.node_cores
      memory        = var.node_memory
      core_fraction = 100
    }

    boot_disk {
      type = "network-ssd"
      size = 100
    }

    network_interface {
      subnet_ids         = values(var.private_subnet_ids)
      security_group_ids = [var.k8s_sg_id]
      nat                = false
    }

    container_runtime {
      type = "containerd"
    }
  }

  scale_policy {
    auto_scale {
      min     = var.node_count
      max     = var.node_count * 3
      initial = var.node_count
    }
  }

  allocation_policy {
    dynamic "location" {
      for_each = ["ru-central1-a", "ru-central1-b", "ru-central1-d"]
      content {
        zone = location.value
      }
    }
  }

  maintenance_policy {
    auto_upgrade = true
    auto_repair  = true
  }
}
