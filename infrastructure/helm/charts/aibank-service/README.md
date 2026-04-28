# aibank-service

Generic, reusable Helm chart for any stateless AIbank backend microservice.

This chart is the workhorse of the platform: it renders a `Deployment +
Service + ConfigMap + ServiceAccount + HPA + NetworkPolicy + ServiceMonitor +
PodDisruptionBudget` for any of the 18+ Go services in `services/`.
The umbrella charts `platform-saas/` and `platform-onprem/` declare it as a
sub-chart with multiple `alias:` entries (one per service) and pass per-service
overrides via `values.yaml`.

## When to use

- Stateless Go HTTP/gRPC service
- Standard `/health` and `/ready` endpoints
- Reads config from env / ConfigMap / Secret
- Talks to Postgres / Redis / Kafka / Temporal / other services in-cluster

For services that don't fit (currently only `llm-gateway` due to /healthz path,
optional GPU, and PVC for model cache) — use a dedicated chart.

## Usage as a sub-chart

```yaml
# platform-saas/Chart.yaml
dependencies:
  - name: aibank-service
    alias: tenant-service
    version: "0.1.0"
    repository: "file://../charts/aibank-service"
```

```yaml
# platform-saas/values.yaml
tenant-service:
  image:
    repository: registry.aibank.local/tenant-service
    tag: "0.1.0"
  service:
    port: 8080
  env:
    - name: DATABASE_URL
      valueFrom:
        secretKeyRef:
          name: tenant-service-db
          key: url
```

## Standalone usage

```bash
helm install tenant-service ./charts/aibank-service \
  --namespace platform-saas \
  --set image.repository=registry.aibank.local/tenant-service \
  --set image.tag=0.1.0 \
  --set service.port=8080
```

## Key values

| Value | Default | Notes |
|---|---|---|
| `image.repository` | `registry.aibank.local/aibank-service` | Override per service |
| `image.tag` | `""` (= `.Chart.AppVersion`) | Prefer `image.digest` in prod |
| `image.digest` | `""` | When set, takes precedence over `tag` |
| `replicaCount` | `2` | Ignored when `hpa.enabled=true` |
| `service.port` | `8080` | **Override per service** |
| `resources.limits.cpu` | `500m` | |
| `resources.limits.memory` | `512Mi` | |
| `resources.requests.cpu` | `200m` | |
| `resources.requests.memory` | `256Mi` | |
| `probes.liveness.httpGet.path` | `/health` | Override e.g. `/healthz` |
| `probes.readiness.httpGet.path` | `/ready` | |
| `securityContext.runAsUser` | `65532` | distroless `nonroot` UID |
| `networkPolicy.enabled` | `false` | Enable in SaaS / on-prem overlays |
| `serviceMonitor.enabled` | `false` | Requires Prometheus Operator |
| `hpa.enabled` | `false` | autoscaling/v2 |
| `podDisruptionBudget.enabled` | `false` | Recommend `minAvailable: 1` in prod |

See `values.yaml` for the full annotated set.

## Validating

```bash
helm lint     ./charts/aibank-service
helm template ./charts/aibank-service \
  --set service.port=8080 | kubectl apply --dry-run=client -f -
```

## Notes

- The chart targets Kubernetes 1.28+ (`autoscaling/v2`,
  `policy/v1.PodDisruptionBudget`, `networking.k8s.io/v1.NetworkPolicy`).
- ServiceMonitor uses `monitoring.coreos.com/v1` from the kube-prometheus-stack.
- Image pinning: prefer `image.digest` over `image.tag` for reproducible
  deployments. The on-prem bundle (ADR-0009) pins by digest in the manifest.
