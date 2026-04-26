output "cluster_id" { value = yandex_mdb_postgresql_cluster.main.id }
output "host"       { value = yandex_mdb_postgresql_cluster.main.host[0].fqdn }
output "database"   { value = yandex_mdb_postgresql_database.aibank.name }
output "user"       { value = yandex_mdb_postgresql_user.aibank.name }
