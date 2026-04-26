terraform {
  backend "s3" {
    # Yandex Object Storage as Terraform backend
    endpoint = "https://storage.yandexcloud.net"
    bucket   = "aibank-terraform-state"
    key      = "terraform.tfstate"
    region   = "ru-central1"

    skip_region_validation      = true
    skip_credentials_validation = true
    skip_requesting_account_id  = true
    skip_s3_checksum            = true
  }
}

provider "yandex" {
  token     = var.yc_token
  cloud_id  = var.yc_cloud_id
  folder_id = var.yc_folder_id
  zone      = var.yc_zone
}

module "vpc" {
  source      = "./modules/vpc"
  project     = var.project
  environment = var.environment
}

module "kubernetes" {
  source             = "./modules/kubernetes"
  project            = var.project
  environment        = var.environment
  folder_id          = var.yc_folder_id
  network_id         = module.vpc.network_id
  private_subnet_ids = module.vpc.private_subnet_ids
  k8s_sg_id          = module.vpc.k8s_sg_id
  node_platform      = var.k8s_node_platform
  node_cores         = var.k8s_node_cores
  node_memory        = var.k8s_node_memory
  node_count         = var.k8s_node_count
}

module "postgresql" {
  source      = "./modules/postgresql"
  project     = var.project
  environment = var.environment
  network_id  = module.vpc.network_id
  subnet_ids  = module.vpc.private_subnet_ids
  pg_version  = var.pg_version
  disk_size   = var.pg_disk_size
  db_password = var.db_password
}

module "storage" {
  source      = "./modules/storage"
  project     = var.project
  environment = var.environment
  folder_id   = var.yc_folder_id
}
