# ADR-0014: mTLS между сервисами через Istio + AuthorizationPolicy

* **Дата**: 2026-04-28 (draft, ждёт DevOps + Security review)
* **Статус**: Proposed
* **Owner**: DevOps + Security

## Контекст

`docs/pilot-readiness.md` § 1.4 фиксирует mTLS между сервисами как
**жёсткий блокер пилота**. Сейчас inter-service traffic в кластере
proceeds открытым TCP, что нарушает принцип zero-trust:

* Pod-to-pod трафик не зашифрован — sniff в network namespace раскрывает
  payload (включая ПДн клиентов через document-service / client-service).
* Нет workload identity — любой pod в namespace может звать api-gateway,
  audit-service и т.д. без cryptographic verification of caller.
* Нет defence-in-depth: одного network policy недостаточно (атакующий с
  доступом к kubelet может ходить в обход).

ИБ-аудит банка-партнёра обязательно потребует zero-trust at runtime.

## Решение

### Service mesh: Istio (vs Linkerd, Cilium)

| Mesh | Зрелость | Сложность ops | Совместимость с on-prem |
|------|----------|---------------|--------------------------|
| **Istio** | High (CNCF graduated) | Высокая, но масштабируема | Полная (помещается в air-gapped bundle) |
| Linkerd | High | Низкая | Хорошая, но workload identity слабее |
| Cilium | Mid (mesh mode новый) | Mid; уровень CNI | Зависит от kernel features |

**Рекомендация**: Istio. Он формализован для ФИНтеха, имеет audit-дружественные
телеметрии, и поддерживает SPIFFE/SPIRE identity (нужно для UC ФНС-договора
mTLS-cert аутентификации в перспективе).

### Sidecar injection: per-namespace label

Включаем sidecar injection точечно через label `istio-injection=enabled` на
namespace'ах AIbank-сервисов (`aibank-services`, `aibank-bff`, `aibank-ai`).
**Не** включаем для `vault`, `kafka`, `postgres` — Istio mesh внутри stateful
служб может усложнить debugging без явной выгоды.

### PeerAuthentication: STRICT mTLS mesh-wide

```yaml
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: istio-system
spec:
  mtls:
    mode: STRICT
```

Любой pod без Istio-sidecar получит connection refused. Это форсит миграцию
всех клиентов в mesh — нет «полу-режима» (PERMISSIVE) в production.

### AuthorizationPolicy: explicit allow-list

Default-deny на mesh-wide уровне + per-service allow rules:

```yaml
# api-gateway → может вызывать любой backend в aibank-services
# bff-onboarding → может вызывать только: tenant, identity, document, risk-engine
# bff-admin → может вызывать всё, включая audit/billing (admin RBAC)
# audit-service → принимает только от services в aibank-services namespace,
#                 и от vault для seal events
```

Каждый сервис получает SPIFFE identity вида:
`spiffe://aibank.local/ns/aibank-services/sa/<service-name>`. Это позволяет
строить mTLS-based RBAC, а не network-based (IP-меняющиеся pods).

### NetworkPolicy как defence-in-depth

Даже с Istio оставляем k8s NetworkPolicy:
* L3/L4 фильтр — защита если Istio sidecar упал/обошёлся.
* SecurityContext: `runAsNonRoot`, `readOnlyRootFs` для всех AIbank-pods.
* Egress из mesh строго whitelist'ный (Vault, Postgres, Kafka).

### Cert lifecycle

* Istio CA (Citadel) генерит leaf-сертификаты автоматически с TTL 24h.
* Root CA — раз в 5 лет (управляется Istio).
* Для on-prem банков: рассмотреть SPIRE с банк-CA вместо Citadel
  (Open question № 3).

## Альтернативы (отклонены)

* **TLS на уровне сервисов через Go crypto/tls без mesh** — каждый
  сервис должен сам управлять сертификатами, ротацией, mTLS handshake'ом.
  Поддерживать 21 сервис вручную → high error rate в безопасности.
* **Нативный k8s NetworkPolicy без шифрования** — фильтрация на L3/L4
  но трафик в pod-network в открытом виде. Не проходит ИБ-аудит банков.
* **Нет mTLS, доверие к network periметру** — современный zero-trust
  model'я этого не допускает (NIST SP 800-207).

## Последствия

* **Lead-time**: 1-2 недели DevOps на разворачивание + 1 неделя
  интеграционных тестов.
* **Cost**: ~+50-150 MB RAM на pod (sidecar Envoy) — для 21 сервиса +
  6 AI-агентов ~3-4 GB cluster overhead.
* **Latency**: +1-3ms per hop (mTLS handshake кэшируется, обновляется на
  rotation).
* **Operations**: добавляется Istio control plane (istiod) — отдельный
  вектор oncall + monitoring.
* **Audit**: каждое connection event пишется в Istio audit-log, можно
  пайпить в audit-service.

## Open questions для DevOps + Security

1. **Версия Istio**: 1.22+ stable или Istio Ambient (sidecar-less)?
   Ambient проще, но новее (GA 2024-Q1). Для 2026-04 — обе зрелые.
2. **Telemetry**: Istio metrics → Prometheus или прямо в VictoriaMetrics?
   Telemetry v2 эффективнее, но требует config tuning.
3. **CA для on-prem банков**: Citadel-default или SPIRE с банк-выданным CA?
   ФСТЭК-сертифицированные KMS могут быть требованием.
4. **Egress traffic**: Istio egress gateway или прямой выход через
   k8s NetworkPolicy? Egress gateway даёт audit, но добавляет hop.
5. **mTLS для legacy sidecars**: ABS-адаптеры используют SOAP — wrap'ать
   через Istio sidecar или native cgo?

Требуется DevOps + Security review.
