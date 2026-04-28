# vault — Helm chart для AIbank

3-узловой Vault HA на Integrated Raft storage. Skeleton с разумными
defaults; production-tuning требует DevOps decisions per **ADR-0013**.

## Установка

```bash
helm install vault ./infrastructure/helm/charts/vault \
  --namespace vault --create-namespace \
  --values values-prod.yaml
```

Пример `values-prod.yaml` для Yandex Cloud:

```yaml
replicaCount: 3
persistence:
  storageClass: "yc-network-ssd"
  size: 20Gi
autoUnseal:
  provider: yandexKMS
  yandexKMS:
    keyId: "abjxxx..."
tls:
  enabled: true
  certSecret: vault-tls
ingress:
  enabled: false  # Vault только внутри кластера
```

## Bootstrap (одноразово после первой установки)

```bash
# 1. Init: получить unseal-keys (Shamir 5-of-3) и initial root token.
kubectl exec -n vault vault-0 -- vault operator init \
    -key-shares=5 -key-threshold=3

# 2. Если provider=yandexKMS — auto-unseal сработает при следующем рестарте.
#    Если provider=shamir — unseal вручную на каждом узле:
kubectl exec -n vault vault-0 -- vault operator unseal <key1>
kubectl exec -n vault vault-0 -- vault operator unseal <key2>
kubectl exec -n vault vault-0 -- vault operator unseal <key3>
# (повторить для vault-1 и vault-2)

# 3. Бутстрап Raft cluster — leader vault-0 уже active, остальные join'ятся.

# 4. Включить AppRole auth + KV v2.
vault auth enable approle
vault secrets enable -version=2 -path=secret kv

# 5. Создать роль для каждого AIbank-сервиса.
vault write auth/approle/role/audit-service \
    token_ttl=1h \
    token_max_ttl=24h \
    policies=audit-service

# Сохранить role-id и secret-id для k8s Secret / Vault Agent (см. ADR-0013).
```

## Production checklist

- [ ] `tls.enabled=true` с реальными сертификатами через cert-manager
- [ ] `autoUnseal.provider` ≠ `shamir` (yandexKMS / hsm для on-prem)
- [ ] PVC encryption (cluster-level или Vault transit-seal)
- [ ] Snapshots cron job → encrypted S3 / object storage
- [ ] Audit-device включён (logs → audit-service kafka topic)
- [ ] Pen-test перед production (см. pilot-readiness § 1.4)
- [ ] PagerDuty alert на `vault.sealed` или `vault.standby_count < 2`

## Связанные документы

- ADR-0013 — Vault HA topology (этот chart реализует решения)
- packages/secrets/approle.go — клиентская часть (AppRole login + auto-renew)
- docs/security-architecture.md § 6.2 — keyholders для on-prem
- docs/runbooks/vault-key-rotation.md — TBD в Цикле 9

## Тесты

```bash
helm template vault ./infrastructure/helm/charts/vault | kubectl apply --dry-run=server -f -
helm lint ./infrastructure/helm/charts/vault
```
