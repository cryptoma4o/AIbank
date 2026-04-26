resource "yandex_vpc_network" "main" {
  name = "${var.project}-${var.environment}-vpc"
}

resource "yandex_vpc_subnet" "public" {
  for_each = toset(["ru-central1-a", "ru-central1-b", "ru-central1-d"])

  name           = "${var.project}-${var.environment}-public-${each.key}"
  zone           = each.key
  network_id     = yandex_vpc_network.main.id
  v4_cidr_blocks = [
    each.key == "ru-central1-a" ? "10.0.0.0/24" :
    each.key == "ru-central1-b" ? "10.0.1.0/24" : "10.0.2.0/24"
  ]
}

resource "yandex_vpc_subnet" "private" {
  for_each = toset(["ru-central1-a", "ru-central1-b", "ru-central1-d"])

  name           = "${var.project}-${var.environment}-private-${each.key}"
  zone           = each.key
  network_id     = yandex_vpc_network.main.id
  v4_cidr_blocks = [
    each.key == "ru-central1-a" ? "10.1.0.0/24" :
    each.key == "ru-central1-b" ? "10.1.1.0/24" : "10.1.2.0/24"
  ]
  route_table_id = yandex_vpc_route_table.nat.id
}

resource "yandex_vpc_gateway" "nat" {
  name = "${var.project}-${var.environment}-nat-gw"
  shared_egress_gateway {}
}

resource "yandex_vpc_route_table" "nat" {
  name       = "${var.project}-${var.environment}-nat-rt"
  network_id = yandex_vpc_network.main.id

  static_route {
    destination_prefix = "0.0.0.0/0"
    gateway_id         = yandex_vpc_gateway.nat.id
  }
}

resource "yandex_vpc_security_group" "k8s_nodes" {
  name       = "${var.project}-${var.environment}-k8s-nodes"
  network_id = yandex_vpc_network.main.id

  ingress {
    protocol       = "ANY"
    description    = "Allow all intra-cluster traffic"
    v4_cidr_blocks = ["10.0.0.0/8"]
  }

  egress {
    protocol       = "ANY"
    description    = "Allow all outbound"
    v4_cidr_blocks = ["0.0.0.0/0"]
  }
}
