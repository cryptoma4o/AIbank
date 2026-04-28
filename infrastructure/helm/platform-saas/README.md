# platform-saas

Umbrella Helm chart for **SaaS deployment** of the AIbank platform.

Bundles all stateless backend services as aliased dependencies on the generic
`aibank-service` chart, plus the dedicated `llm-gateway` chart. The stateful
layer (Postgres, Kafka, Redis, MinIO, Temporal) is **NOT** part of this chart
in SaaS — it is provided by managed cloud services (Yandex Cloud / VK Cloud).
Connection details come from pre-created Secrets.

## Layout

```
platform-saas/
├── Chart.yaml         # 18 dependencies (17 aliased aibank-service + 1 llm-gateway)
├── values.yaml        # SaaS-defaults: HPA on, NetworkPolicy on, ServiceMonitor on
└── README.md
```

## Prerequisites

- Kubernetes 1.28+
- kube-prometheus-stack (for ServiceMonitor / Prometheus rules)
- Istio with `platform-mesh` namespace (per ADR-0009 § Part 1)
- External Secrets Operator (or equivalent) — Secrets must exist before install:
  - `aibank-postgres-credentials`
  - `aibank-redis-credentials`
  - `aibank-kafka-credentials`
  - `aibank-minio-credentials`
  - `aibank-jwt-keys`
  - `aibank-llm-providers`
  - `aibank-registry-creds`

## Install

```bash
cd infrastructure/helm

# Resolve sub-charts on first run / after Chart.yaml changes
helm dependency update platform-saas/

# Render manifests for review (does not deploy)
helm template aibank platform-saas/ \
  --namespace platform \
  -f platform-saas/values.yaml | less

# Lint
helm lint platform-saas/

# Install
helm install aibank platform-saas/ \
  --namespace platform --create-namespace \
  -f platform-saas/values.yaml \
  -f values-prod.yaml         # optional cluster-specific overrides
```

## Per-tenant releases

Per `technical-structure.md` § 10.1 ("K8s namespace per tenant") deploy one
release per tenant:

```bash
helm install aibank-bank-alpha platform-saas/ \
  --namespace tenant-bank-alpha --create-namespace \
  -f platform-saas/values.yaml \
  -f tenants/bank-alpha/values.yaml
```

## Upgrade & blue/green (per ADR-0009)

In SaaS we use rolling updates by default (continuous deployment). When a
release is risky, use the same blue-green pattern as on-prem:

```bash
helm install  aibank-blue  platform-saas/ -n aibank-blue  --create-namespace -f values.yaml --set image.tag=1.4.2
helm install  aibank-green platform-saas/ -n aibank-green --create-namespace -f values.yaml --set image.tag=1.5.0
# switch Istio VirtualService weights from blue→green atomically
kubectl apply -f istio/virtualservice-green.yaml
```

## TODO

- Per-environment `values-{dev,staging,prod}.yaml` (deferred).
- `tools/helm-bake.sh` script generating per-tenant values from
  `configs/tenants/<id>/tenant.yaml` (deferred — depends on ADR-002 schema).
- ArgoCD Application manifest pointing at this chart (deferred to `infrastructure/argocd/`).
