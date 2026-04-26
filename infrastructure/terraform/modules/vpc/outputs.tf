output "network_id"         { value = yandex_vpc_network.main.id }
output "public_subnet_ids"  { value = { for k, v in yandex_vpc_subnet.public : k => v.id } }
output "private_subnet_ids" { value = { for k, v in yandex_vpc_subnet.private : k => v.id } }
output "k8s_sg_id"          { value = yandex_vpc_security_group.k8s_nodes.id }
