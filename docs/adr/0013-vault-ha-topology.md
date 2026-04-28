# ADR-0013: Vault HA topology, auto-unseal и AppRole-аутентификация

* **Дата**: 2026-04-28 (draft, ждёт DevOps review)
* **Статус**: Proposed
* **Owner**: DevOps + Security

## Контекст

`packages/secrets` уже умеет читать KV v2 из Vault, но конфигурация
сейчас однопоточная: один Vault-узел + статический `VAULT_TOKEN`.
Это **жёсткий блокер пилота** (см. `docs/pilot-readiness.md` § 1.4):

* Single point of failure (один pod / VM Vault'а — потеря всех секретов).
* `VAULT_TOKEN` нельзя ротировать без перезапуска всех сервисов.
* Vault unseal — ручная операция (Shamir 5-of-3 или 3-of-2), что
  блокирует автоматический recovery после crash'а.
* Нет formalized процедуры master-key rotation.

Пилот с банком требует HA (RPO=0 для секретов), auto-unseal через
KMS, и AppRole flow (per-pod ephemeral credentials с авто-renewal).

## Решение

### Топология: Vault Raft HA, 3 узла

```
                 ┌──────────────┐
                 │  vault-0     │ ← active leader
                 └──────┬───────┘
                        │ Raft replication
            ┌───────────┼───────────┐
            ▼                       ▼
     ┌──────────────┐        ┌──────────────┐
     │  vault-1     │        │  vault-2     │
     └──────────────┘        └──────────────┘
       standby                  standby
```

* **Storage backend**: Integrated Raft (Vault built-in, без внешней БД).
  Альтернативы (Consul, etcd) добавляют сложность.
* **PVC** на каждый узел: 10Gi (encrypted) для raft data.
* **PodDisruptionBudget**: minAvailable=2 — при rolling-update теряем
  только один узел, остальные обслуживают трафик.
* **anti-affinity**: pods на разных nodes (`requiredDuringScheduling`),
  чтобы DR при потере node не унёс HA.

### Auto-unseal: облако-зависимое решение

Pre-MVP мы **не привязываемся** к одному cloud-провайдеру. Choice — за
DevOps:

| Provider | Auto-unseal mechanism | Lead-time |
|----------|----------------------|-----------|
| Yandex Cloud | KMS (символ. ключ + IAM) | 1 нед |
| Selectel | Shamir + Vault Agent (manual unseal на crash) | 0 нед, но требует on-call |
| On-prem (для банков) | HSM (КриптоПро HSM) или Shamir + air-gapped key | 4-6 нед — синхронизировано с УКЭП-лицензиями |

**Рекомендация**: для SaaS — Yandex KMS (нативная интеграция через
`seal "yandexcloudkms"` block). Для on-prem — Shamir с keyholder'ами
у банка (key sharing per security-architecture § 6.2).

### AppRole flow: ephemeral per-service credentials

Заменяем `VAULT_TOKEN` на per-pod AppRole login:

```
1. DevOps создаёт AppRole "audit-service" с TTL=1h, max-TTL=24h.
2. AppRole role_id попадает в k8s ConfigMap (не секрет — public-id).
3. AppRole secret_id попадает через одну из:
   a) k8s Secret + Vault Agent sidecar (рекомендация для SaaS)
   b) CSI driver с SPIFFE auth (рекомендация для on-prem k8s)
   c) Vault Agent Init container с одноразовым wrapping_token
4. Сервис на старте: secrets.NewVaultProviderAppRole(roleID, secretID)
   → token, TTL 1h.
5. Background goroutine renew'ит token каждые 30 мин.
```

См. `packages/secrets/approle.go` (новый, в этом PR).

### Master key rotation: процедура

* Раз в год — automated runbook (см. `docs/runbooks/vault-key-rotation.md`,
  to-be-written в Цикле 9).
* `vault operator rekey -init -key-shares=5 -key-threshold=3` →
  новые keyholders → пересборка unseal keys.
* Audit-trail в audit-service event_type=`vault.master_key_rotated`.

## Альтернативы (отклонены)

* **HashiCorp Cloud Platform Vault** — managed offering, но требует
  internet connection из cluster, что нарушает on-prem requirement
  банков (§ 4.2 security-architecture: outbound whitelist).
* **AWS Secrets Manager + KMS** — не подходит для on-prem; SaaS-only.
* **Self-rolled secrets через k8s Secret + Sealed Secrets** — без
  rotation/audit, не соответствует 152-ФЗ требованиям к управлению ПДн.

## Последствия

* **Lead-time**: 1-2 недели DevOps + 1 неделя Security review +
  1 неделя интеграционных тестов.
* **Cost**: ~3× CPU/RAM vs single-node + KMS-billing.
* **Operations**: добавляется PagerDuty rotation для Vault (если down >
  N min — escalate).
* **Audit**: каждое чтение секрета пишется в audit-service.

## Open questions для DevOps

1. **KMS для SaaS**: Yandex Cloud KMS ОК или альтернатива?
2. **PVC encryption**: cluster-level (CSI driver flag) или Vault-level
   (encrypted Raft via storage encryption)?
3. **Network**: Vault expose только intra-cluster или через mTLS Ingress?
4. **Backup**: Raft snapshots каждые N часов в encrypted S3-бакет —
   подходит ли retention 30 дней?
5. **On-prem банки**: HSM КриптоПро или fallback на Shamir с банковскими
   keyholder'ами?

Требуется DevOps + Security review до старта реализации в production.
