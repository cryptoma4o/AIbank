# redis (wrapper chart)

Values-only chart for Redis. Wraps the upstream **Bitnami `redis`** chart.

```bash
helm install redis bitnami/redis \
  --namespace platform-data --create-namespace \
  -f infrastructure/helm/charts/redis/values.yaml
```

Required Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aibank-redis-credentials
  namespace: platform-data
type: Opaque
stringData:
  redis-password: <strong-password>
```

## TODO

- TLS in-transit via cert-manager Issuer — deferred.
- Redis Sentinel / Cluster mode evaluation for high-cardinality tenants — deferred.
