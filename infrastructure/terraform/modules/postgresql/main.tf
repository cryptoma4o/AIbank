resource "yandex_mdb_postgresql_cluster" "main" {
  name        = "${var.project}-${var.environment}-pg"
  environment = var.environment == "prod" ? "PRODUCTION" : "PRESTABLE"
  network_id  = var.network_id

  config {
    version = var.pg_version
    resources {
      resource_preset_id = var.environment == "prod" ? "s3-c2-m8" : "s2-micro"
      disk_type_id       = "network-ssd"
      disk_size          = var.disk_size
    }
    postgresql_config = {
      max_connections                = 400
      shared_buffers                 = 536870912  # 512MB
      work_mem                       = 16777216   # 16MB
      log_min_duration_statement     = 1000       # log queries > 1s
    }
  }

  dynamic "host" {
    for_each = ["ru-central1-a", "ru-central1-b"]
    content {
      zone      = host.value
      subnet_id = var.subnet_ids[host.value]
    }
  }

  maintenance_window {
    type = "WEEKLY"
    day  = "SUN"
    hour = 3
  }
}

resource "yandex_mdb_postgresql_database" "aibank" {
  cluster_id = yandex_mdb_postgresql_cluster.main.id
  name       = "aibank"
  owner      = yandex_mdb_postgresql_user.aibank.name
}

resource "yandex_mdb_postgresql_user" "aibank" {
  cluster_id = yandex_mdb_postgresql_cluster.main.id
  name       = "aibank"
  password   = var.db_password
  conn_limit = 100
}
