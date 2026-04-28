# minio (wrapper chart)

Values-only chart for **MinIO** (S3-compatible object storage). Wraps the
upstream **Bitnami `minio`** chart.

Used for documents, audit log archives, and on-prem LLM model weights.

```bash
helm install minio bitnami/minio \
  --namespace platform-data --create-namespace \
  -f infrastructure/helm/charts/minio/values.yaml
```

In SaaS prefer **managed S3** (Yandex Cloud Object Storage / Selectel S3) and
do NOT deploy this chart — only configure endpoint + credentials via Secret.

## TODO

- Per-tenant lifecycle policies (audit-archive 5y retention, docs 7y) —
  deferred to `services/tenant-service` provisioning logic.
- Cross-region replication for SaaS — deferred.
