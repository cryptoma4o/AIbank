resource "yandex_dns_zone" "aibank" {
  name        = "${var.project}-${var.environment}-zone"
  zone        = "${var.domain}."
  public      = true
  description = "AIbank ${var.environment} DNS zone"
}

resource "yandex_dns_recordset" "api_wildcard" {
  zone_id = yandex_dns_zone.aibank.id
  name    = "*.api.${var.domain}."
  type    = "A"
  ttl     = 300
  data    = [var.ingress_ip]
}
