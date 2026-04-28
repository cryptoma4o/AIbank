# platform-onprem

Umbrella Helm chart for **on-prem deployment** of the AIbank platform.

Same set of stateless services as `platform-saas`, but with on-prem defaults:

| Aspect | SaaS | On-Prem |
|---|---|---|
| `replicaCount` (default) | 2–3 | **1** (api-gateway/audit-service: 2) |
| `hpa.enabled` | true | **false** (predictable capacity) |
| `networkPolicy.enabled` | true | **true** (also default-deny) |
| `serviceMonitor.enabled` | true | true |
| Image registry default | `registry.aibank.local` | `bank-registry.local` (private mirror) |
| Resource budgets | full | **~30% lower** |

Stateful infra (Postgres, Kafka, Redis, MinIO, Temporal) is **not bundled** —
banks bring their own. Use the wrapper charts under
`infrastructure/helm/charts/{postgres,redis,kafka,minio,temporal}/` if needed,
deployed separately into `platform-data`.

## Prerequisites

- Kubernetes 1.28+ (vanilla, OKD, or Deckhouse)
- Istio 1.20+ in `platform-mesh` namespace (for blue-green via VirtualService)
- kube-prometheus-stack (or bank-equivalent ServiceMonitor controller)
- The following Secrets must exist in the target namespace before install:
  - `aibank-postgres-credentials`
  - `aibank-redis-credentials`
  - `aibank-kafka-credentials`
  - `aibank-minio-credentials`
  - `aibank-jwt-keys`
  - `aibank-llm-providers`
  - `aibank-registry-creds` (private registry pull)

## Install (blue/green per ADR-0009)

```bash
cd infrastructure/helm

# Initial deploy as 'blue'
helm dependency update platform-onprem/
helm install aibank platform-onprem/ \
  --namespace platform-blue --create-namespace \
  -f platform-onprem/values.yaml \
  -f values-bank-alpha.yaml \
  --set global.imageTag=1.4.2

# Quarterly upgrade — deploy 'green' alongside
helm install aibank platform-onprem/ \
  --namespace platform-green --create-namespace \
  -f platform-onprem/values.yaml \
  -f values-bank-alpha.yaml \
  --set global.imageTag=1.5.0

# Smoke-test green via private VirtualService
# (see ADR-0009 § Part 3 — /v1/__smoke routed to green)

# Atomic switch via Istio VirtualService weights:
kubectl apply -f istio/platform-vs-green.yaml

# Rollback (5–10 minutes):
kubectl apply -f istio/platform-vs-blue.yaml
```

## Validation

```bash
# Helm chart linter
helm lint platform-onprem/

# Render full manifest set
helm template aibank platform-onprem/ -f platform-onprem/values.yaml | kubeval --kubernetes-version 1.30.0

# Dry-run against a real cluster
helm upgrade --install aibank platform-onprem/ --dry-run --debug \
  -n platform-blue --create-namespace
```

## TODO

- Bitnami chart pinning for Postgres/Kafka/Redis/MinIO (deferred — needs vetting cycle and a private mirror).
- `tenant-cli` Job/CronJob templates (deferred — see ADR-0009 § Part 6).
- Vault Agent sidecar via `templates/_vault-injector.tpl` (deferred).
- cert-manager `Issuer` + Certificate templates for in-cluster TLS (deferred).
- `db-migrator` Job template (one-shot pre-upgrade) — deferred to ADR-0005 follow-up.
