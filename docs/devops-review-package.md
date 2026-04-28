<!-- Parent: ./AGENTS.md -->
<!-- Generated: 2026-04-28 -->

# DevOps Review Package для AIbank

Свод материалов для DevOps-команды банка-партнёра / внутреннего DevOps:
два готовых-к-обсуждению ADR + готовые helm chart skeletons.
Цель — сократить итерацию «вопрос → ответ» до 1 встречи на ADR.

**Время на review**: 60-90 минут на каждый ADR + 1 встреча для синтеза.
**Дедлайн до пилота**: 4-6 недель (см. `docs/pilot-readiness.md`).

---

## Что в этом пакете

| Артефакт | Назначение | Файл |
|----------|-----------|------|
| ADR-0013 Vault HA | Архитектурное решение для secrets management | [docs/adr/0013-vault-ha-topology.md](./adr/0013-vault-ha-topology.md) |
| ADR-0014 Istio mTLS | Архитектурное решение для inter-service security | [docs/adr/0014-mtls-istio.md](./adr/0014-mtls-istio.md) |
| Vault Helm chart | Готовый-к-deploy skeleton с разумными defaults | [infrastructure/helm/charts/vault/](../infrastructure/helm/charts/vault/) |
| Istio mesh chart | AIbank-policies (PeerAuth, AuthZ, NetworkPolicy) | [infrastructure/helm/charts/istio-mesh/](../infrastructure/helm/charts/istio-mesh/) |
| AppRole client | Go-клиент с auto-renewal для всех сервисов | [packages/secrets/approle.go](../packages/secrets/approle.go) |

---

## Перед review — self-validation на стороне DevOps

```bash
# 1. Helm chart syntax check
helm lint infrastructure/helm/charts/vault
helm lint infrastructure/helm/charts/istio-mesh

# 2. Render manifests без apply
helm template vault infrastructure/helm/charts/vault > /tmp/vault.yaml
helm template aibank-mesh infrastructure/helm/charts/istio-mesh > /tmp/mesh.yaml

# 3. K8s schema validation (без cluster connect)
kubectl --client=true apply --dry-run=client -f /tmp/vault.yaml
kubectl --client=true apply --dry-run=client -f /tmp/mesh.yaml

# 4. AppRole tests
cd packages/secrets && go test -count=1 -v -run TestAppRole
```

Если что-то падает — запиши в комментарий ниже, обсудим.

---

## Ключевые решения, требующие подтверждения

### 1. Vault: KMS-провайдер для auto-unseal (ADR-0013 § 2)

**Опции** (выберите один):

| Вариант | Lead-time | Cost | Подходит для |
|---------|-----------|------|--------------|
| ☐ Yandex Cloud KMS | 1 нед | низкий | SaaS-tenants на Yandex |
| ☐ AWS KMS | 1 нед | средний | если есть AWS account |
| ☐ Self-hosted HSM (КриптоПро) | 4-6 нед | высокий | on-prem банки |
| ☐ Shamir с keyholder'ами банка | 0 нед, но manual unseal | нулевой | on-prem fallback |

**Текущий default в chart**: `shamir` (manual unseal). Production требует
override через `values-prod.yaml`.

### 2. Vault: PVC encryption (ADR-0013 § 2)

| Опция | Pros | Cons |
|-------|------|------|
| ☐ Cluster-level (CSI driver flag) | прозрачно для приложения | зависит от CSI vendor |
| ☐ Vault transit-seal | вне зависимости от CSI | дополнительный component |

### 3. Vault: backup retention (ADR-0013 § Open question 4)

Raft snapshots каждые 6h в encrypted S3. **Retention?**

| Вариант | Объём (примерно за 90 дней) |
|---------|------------------------------|
| ☐ 30 дней | ~10 GB |
| ☐ 90 дней | ~30 GB |
| ☐ 1 год + glacier-tier для старого | ~50 GB hot + 100 GB cold |

### 4. Istio: версия (ADR-0014 § Open question 1)

| Версия | Зрелость | Sidecar overhead | Когда выбирать |
|--------|----------|------------------|----------------|
| ☐ Istio 1.22 sidecar | High | ~50-150 MB / pod | proven, для всех CNI |
| ☐ Istio Ambient (sidecar-less) | GA 2024-Q1 | ~10-30 MB / node | если CNI поддерживает (Calico, Cilium) |

**Рекомендация AIbank**: 1.22 sidecar для пилота — больше production-runs, проще
поддерживать. Ambient — рассмотреть после первого пилота.

### 5. Istio: telemetry exporter (ADR-0014 § Open question 2)

| Вариант | Где live |
|---------|----------|
| ☐ Prometheus + Grafana | стандарт |
| ☐ VictoriaMetrics | если у вас уже есть VM cluster |
| ☐ Audit-service (Kafka) | для compliance audit-trail (по 152-ФЗ) |

Можно несколько одновременно.

### 6. Istio: CA для on-prem банков (ADR-0014 § Open question 3)

| Опция | Подходит когда |
|-------|----------------|
| ☐ Citadel default (self-signed) | dev/staging, SaaS pilot |
| ☐ SPIRE с банк-выданным CA | on-prem банки с собственным PKI |
| ☐ ФСТЭК-сертифицированный KMS | если требование ФСТЭК-аудита |

### 7. Istio: egress gateway (ADR-0014 § Open question 4)

| Опция | Pros | Cons |
|-------|------|------|
| ☐ Egress gateway (через istio-egress) | централизованный audit, шифрование к external | extra hop, +complexity |
| ☐ Прямой egress через NetworkPolicy whitelist | проще, без hop | нет mTLS на external traffic |

---

## Как структурировать review-meeting

### Часть 1: Vault HA (45 мин)

1. **5 мин**: Founder показывает контекст pilot-readiness блокеров
2. **15 мин**: Walkthrough ADR-0013 — топология, auto-unseal, AppRole
3. **20 мин**: Open questions 1-4 (KMS, PVC encryption, backup retention)
4. **5 мин**: Назначение action items (deadline до production rollout)

### Часть 2: Istio mTLS (45 мин)

1. **15 мин**: Walkthrough ADR-0014 — mesh, AuthZ, NetworkPolicy
2. **20 мин**: Open questions 1-5 (версия, telemetry, CA, egress, ABS legacy)
3. **5 мин**: Action items
4. **5 мин**: Прогон self-validation команд (helm lint / kubectl dry-run)

### Часть 3: Synthesis (30 мин)

- Заполнить чек-боксы выше совместно
- Записать решения в `docs/adr/0013-vault-ha-topology.md` и
  `0014-mtls-istio.md` (статус Proposed → Accepted)
- Создать issues в трекере на каждое action item с deadline

---

## Action items template (для tracker)

После meeting'а создать issues:

| ID | Title | Owner | Due | Acceptance |
|----|-------|-------|-----|------------|
| INFRA-101 | Provision KMS instance | DevOps | +1 нед | Vault auto-unseal работает |
| INFRA-102 | Configure CSI с encryption | DevOps | +1 нед | PVC inspect показывает encrypted=true |
| INFRA-103 | Setup S3 bucket для Vault snapshots | DevOps | +1 нед | CronJob с тестовым snapshot |
| INFRA-104 | Pen-test провайдер найден | Security | +2 нед | контракт подписан |
| INFRA-105 | Istio control plane install (staging) | DevOps | +1 нед | istioctl version в кластере |
| INFRA-106 | Apply mesh chart (staging) | DevOps | +2 нед | curl без sidecar = denied |
| INFRA-107 | Telemetry экспорт настроен | DevOps | +2 нед | Grafana dashboard видит p99 latency |
| INFRA-108 | Audit-trail Vault → audit-service | Backend | +3 нед | event_type=vault.* появляется в audit |
| INFRA-109 | DR drill: Vault crash → auto-unseal | DevOps + Security | +3 нед | runbook прошёл успешно |
| INFRA-110 | Migration plan для production-ABS | DevOps + Backend | +4 нед | rollout-document готов |

---

## Что MOJET блокировать review

Если у DevOps нет доступа / опыта по любому из:

- **Cloud KMS API** (Yandex/AWS/GCP) — нужно подключить cloud-инженера
- **Istio production deploy** — рассмотреть консультанта (Istio Steering)
- **СКЗИ КриптоПро integration** — нужен сертифицированный специалист
  (банк-партнёр обычно имеет — спросить напрямую)
- **k8s networking deep-dive** — Cilium / Calico CNI требует специфичных
  знаний

Решение в этих случаях — **не блокировать пилот**, использовать defaults
(Shamir для Vault, Istio sidecar 1.22 default Citadel CA), а консультаций
заказать параллельно.

---

## Контакты и эскалация

- **Технический owner ADR**: AIbank platform team
- **Эскалация по Vault**: Security-инженер (см. AGENTS.md → Зоны
  ответственности)
- **Эскалация по Istio**: DevOps-lead

После review — обновите статусы ADR с Proposed → Accepted (или Rejected
с обоснованием) и закоммитьте.
