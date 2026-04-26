module "vpc" {
  source      = "../../modules/vpc"
  project     = "aibank"
  environment = "prod"
}

module "kubernetes" {
  source             = "../../modules/kubernetes"
  project            = "aibank"
  environment        = "prod"
  folder_id          = var.yc_folder_id
  network_id         = module.vpc.network_id
  private_subnet_ids = module.vpc.private_subnet_ids
  k8s_sg_id          = module.vpc.k8s_sg_id
  node_count         = 3
  node_cores         = 8
  node_memory        = 32
}

module "postgresql" {
  source      = "../../modules/postgresql"
  project     = "aibank"
  environment = "prod"
  network_id  = module.vpc.network_id
  subnet_ids  = module.vpc.private_subnet_ids
  disk_size   = 500
  db_password = var.db_password
}

module "storage" {
  source      = "../../modules/storage"
  project     = "aibank"
  environment = "prod"
  folder_id   = var.yc_folder_id
}

variable "yc_folder_id" { type = string }
variable "db_password"  { type = string; sensitive = true }
