# postgres (wrapper chart)

Recommended **values-only** chart for PostgreSQL.

This chart does **not** carry templates. It exists to:

1. Document the recommended values for the upstream **Bitnami `postgresql`** chart.
2. Provide a stable file path the umbrella charts can reference for `-f` overrides.

In production deployments prefer **managed Postgres** (cloud or bank-provided).
Use this chart only when you must run Postgres in-cluster.

## Apply

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm install pg bitnami/postgresql \
  --namespace platform-data --create-namespace \
  -f infrastructure/helm/charts/postgres/values.yaml
```

Or via OCI mirror (recommended for air-gapped on-prem):

```bash
helm install pg oci://your-mirror.local/bitnamicharts/postgresql \
  --namespace platform-data --create-namespace \
  -f infrastructure/helm/charts/postgres/values.yaml
```

## Required pre-existing Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aibank-postgres-credentials
  namespace: platform-data
type: Opaque
stringData:
  postgres-password: <strong-admin-password>
  aibank-password:   <strong-app-password>
  replication-password: <strong-replication-password>
```

## TODO

- Pin Bitnami chart version explicitly in umbrella `Chart.yaml` (deferred — needs vetting cycle).
- Backup CronJob (pg_basebackup → MinIO) — separate chart `postgres-backup` (deferred).
- TLS in-transit (cert-manager Issuer) — deferred.
