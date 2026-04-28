# istio-mesh — AIbank-specific Istio policies

Применяет mTLS-mesh-policies для AIbank-сервисов поверх установленного
Istio control plane (istiod). См. **ADR-0014** для архитектурных решений.

## Этот chart НЕ устанавливает

* Istio control plane (istiod) — устанавливается через istioctl или
  istio-base/istiod helm-charts.
* CRDs (PeerAuthentication, AuthorizationPolicy) — приходят с istio-base.
* Telemetry / Kiali / Jaeger — отдельные deployments.

## Что делает chart

1. **Namespace labels**: `istio-injection=enabled` на AIbank namespace'ах
   (sidecar injection autonomously происходит при создании pods).
2. **PeerAuthentication STRICT**: только pods с sidecar могут общаться.
3. **AuthorizationPolicy default-deny + explicit allow** rules per
   ADR-0014:
   - api-gateway → backend services
   - bff-onboarding → конкретные backends (least privilege)
   - bff-admin → все (admin RBAC)
   - AI-агенты → llm-gateway
   - audit-service ingress только от AIbank + Vault
4. **NetworkPolicy** как defence-in-depth (L3/L4 если sidecar упал).

## Установка

**Шаг 0**. Установить Istio control plane.

```bash
istioctl install --set profile=default \
    --set meshConfig.accessLogFile=/dev/stdout \
    --set values.global.meshID=aibank \
    --set values.global.network=aibank-network
```

**Шаг 1**. Применить AIbank-специфичные policies.

```bash
helm install aibank-mesh ./infrastructure/helm/charts/istio-mesh \
  --namespace istio-system
```

**Шаг 2**. Создать AIbank namespaces (с sidecar injection).

```bash
kubectl create namespace aibank-services
kubectl create namespace aibank-bff
kubectl create namespace aibank-ai
# Labels добавятся автоматически через chart.
```

**Шаг 3**. Перезапустить AIbank-pods чтобы получить sidecar.

```bash
kubectl rollout restart deployment -n aibank-services
kubectl rollout restart deployment -n aibank-bff
kubectl rollout restart deployment -n aibank-ai
```

## Verification

```bash
# 1. Проверить что pods получили sidecar (2 контейнера: app + istio-proxy).
kubectl get pods -n aibank-services -o jsonpath='{range .items[*]}{.metadata.name}: {.spec.containers[*].name}{"\n"}{end}'

# 2. Проверить что mTLS работает (без sidecar pod не дойдёт).
kubectl run -n default test-curl --rm -it --image=curlimages/curl -- \
    curl http://api-gateway.aibank-services.svc.cluster.local:8000/healthz
# Ожидается connection refused / RBAC: access denied.

# 3. Из namespace AIbank — должно работать.
kubectl run -n aibank-bff test-curl --rm -it --image=curlimages/curl -- \
    curl http://api-gateway.aibank-services.svc.cluster.local:8000/healthz
# Ожидается 200 OK.
```

## Production checklist

- [ ] Istio version pin'ован (≥1.22 stable или Ambient)
- [ ] `peerAuth.mode=STRICT` (после migration period с PERMISSIVE)
- [ ] Все AIbank-pods имеют sidecar (rolling restart выполнен)
- [ ] AuthorizationPolicy rules покрывают все production-сервисы
- [ ] CA для on-prem банков выбран (Citadel default или SPIRE с банк-CA)
- [ ] Telemetry v2 настроен → audit-service / VictoriaMetrics
- [ ] Egress rules через NetworkPolicy whitelist (Vault, Postgres, Kafka)

## Связанные документы

- ADR-0014 — mTLS Istio topology (этот chart реализует решения)
- ADR-0009 — On-prem update strategy (blue-green namespaces через Istio)
- docs/security-architecture.md § 4.2 — outbound whitelist
- docs/pilot-readiness.md § 1.4 — security блокеры пилота

## Ограничения текущего skeleton

* `authz.rules[].target.services` — поддерживает только первый сервис
  (helm template upper) — нужен upgrade для multi-service rules
  (FIXME-комментарий в authorization-policies.yaml).
* Нет SPIRE-integration — только Citadel default identities.
* Нет multi-cluster federation (для DR между регионами).
* Нет egress gateway (open question № 4).
