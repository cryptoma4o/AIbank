# infrastructure/helm

Helm charts for deploying the AIbank platform to Kubernetes.

## Layout

```
infrastructure/helm/
├── charts/                       # individual service / wrapper charts
│   ├── aibank-service/           # GENERIC reusable chart for any Go backend service
│   ├── llm-gateway/              # specific FastAPI chart (port 8100, /healthz)
│   ├── postgres/                 # values-only wrapper for upstream Bitnami
│   ├── redis/                    # values-only wrapper for upstream Bitnami
│   ├── kafka/                    # values-only wrapper for upstream Bitnami (KRaft)
│   ├── minio/                    # values-only wrapper for upstream Bitnami
│   └── temporal/                 # values-only wrapper for temporalio chart
├── platform-saas/                # umbrella chart — SaaS deployment
│   ├── Chart.yaml                # 18 dependencies (17 aliased aibank-service + llm-gateway)
│   ├── values.yaml               # SaaS defaults (HPA on, NetworkPolicy on, etc.)
│   └── README.md
└── platform-onprem/              # umbrella chart — on-prem deployment
    ├── Chart.yaml                # same 18 dependencies as SaaS
    ├── values.yaml               # on-prem defaults (replica=1, no HPA, smaller resources)
    └── README.md
```

## Concepts

### `aibank-service` is the workhorse

A single generic chart deploys **any** stateless AIbank backend service:
`tenant-service`, `audit-service`, `identity-service`, `document-service`,
`onboarding-orchestrator`, `risk-engine`, `billing-service`, `abs-connector`,
`client-service`, `ubo-service`, `bff-onboarding`, `bff-admin`, `api-gateway`,
`notification-service`, `ext-egrul`, `ext-rosfinmon`, `ext-fssp`, plus future
ABS adapters and Go services.

It renders: `Deployment + Service + ConfigMap + ServiceAccount + HPA +
NetworkPolicy + ServiceMonitor + PodDisruptionBudget`.

The umbrella charts (`platform-saas`, `platform-onprem`) declare it 17 times
with different `alias:` entries — each alias becomes a fully independent
release of the same chart with per-service overrides in `values.yaml`.

### `llm-gateway` is special

Different from the Go services (FastAPI/Python, port 8100, `/healthz` instead
of `/health`, optional GPU & PVC for vLLM model cache). It has its own
chart but the same shape (Deployment + Service + ...).

### Wrapper charts under `charts/{postgres,redis,kafka,minio,temporal}/`

These are **values-only** wrappers. They carry a `Chart.yaml` and the
recommended `values.yaml` for the upstream Bitnami / temporalio chart, but
**no `templates/`** — the umbrella charts can later add them as conditional
dependencies. In production the stateful layer is provided by:

- **SaaS** — managed cloud services (Yandex Cloud / VK Cloud).
- **On-prem** — bank-provided Postgres / Kafka / etc., or these wrappers
  installed separately into the `platform-data` namespace per ADR-0009.

## Workflow

### Develop / validate

```bash
cd infrastructure/helm

# Lint individual charts
helm lint charts/aibank-service/
helm lint charts/llm-gateway/

# Resolve sub-chart dependencies (file:// repositories)
helm dependency update platform-saas/
helm dependency update platform-onprem/

# Lint umbrella charts
helm lint platform-saas/
helm lint platform-onprem/

# Render full manifest set for review
helm template aibank platform-saas/   -f platform-saas/values.yaml   | less
helm template aibank platform-onprem/ -f platform-onprem/values.yaml | less

# Schema-validate against Kubernetes API
helm template aibank platform-saas/ | kubeval --kubernetes-version 1.30.0
```

### Deploy — SaaS

```bash
helm install aibank platform-saas/ \
  --namespace aibank-saas --create-namespace \
  -f platform-saas/values.yaml \
  -f values-prod.yaml
```

Per `technical-structure.md` § 10.1, deploy one release per tenant:

```bash
helm install aibank-bank-alpha platform-saas/ \
  --namespace tenant-bank-alpha --create-namespace \
  -f platform-saas/values.yaml \
  -f tenants/bank-alpha/values.yaml
```

### Deploy — on-prem (blue/green per ADR-0009)

```bash
# Initial deploy
helm install aibank platform-onprem/ \
  --namespace platform-blue --create-namespace \
  -f platform-onprem/values.yaml \
  -f values-bank-alpha.yaml \
  --set global.imageTag=1.4.2

# Quarterly upgrade — deploy green alongside blue
helm install aibank platform-onprem/ \
  --namespace platform-green --create-namespace \
  -f platform-onprem/values.yaml \
  -f values-bank-alpha.yaml \
  --set global.imageTag=1.5.0

# Atomically flip the Istio VirtualService weights blue→green
kubectl apply -f istio/platform-vs-green.yaml

# Roll back in 5–10 minutes if needed
kubectl apply -f istio/platform-vs-blue.yaml
```

ADR-0009 details the full procedure, including the `tenant-cli`,
forward-compatible migrations, and the signed air-gapped bundle.

## Conventions

- All charts: version `0.1.0`, `apiVersion: v2`, Kubernetes 1.28+.
- All images pinned by `image.tag` in MVP; production overlays should
  pin by `image.digest` (sha256 from the bundle manifest).
- All Pod containers run as `runAsUser: 65532` (distroless `nonroot` UID).
- Secrets are never inlined in `values.yaml` — only `existingSecret` references.
- Default labels include `app.kubernetes.io/part-of: aibank` for cross-cutting
  selection in alerting rules and NetworkPolicies.

## Status

Pre-MVP (Phase 1 of ADR-0009 implementation roadmap).

| Deliverable | State |
|---|---|
| `aibank-service` generic chart | Done |
| `llm-gateway` chart | Done |
| `platform-saas` umbrella | Done — 18 dependencies |
| `platform-onprem` umbrella | Done — 18 dependencies |
| Wrapper charts (postgres/redis/kafka/minio/temporal) | Done — values-only |
| `tenant-cli` Job templates | Deferred |
| Vault Agent sidecar | Deferred |
| cert-manager Issuer/Certificate templates | Deferred |
| Bitnami chart version pinning | Deferred |
| ArgoCD Application manifests | Deferred — see `infrastructure/argocd/` (not yet created) |

## See also

- `docs/technical-structure.md` § 10 — deployment topology.
- `docs/adr/0009-on-prem-update-strategy.md` — blue/green namespaces, signed bundles.
- `infrastructure/terraform/` — provisions the K8s cluster these charts run on.
- `configs/tenants/_template/` — per-tenant configuration consumed by `tenant-service`.
