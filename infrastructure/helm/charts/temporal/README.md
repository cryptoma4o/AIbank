# temporal (wrapper chart)

Values-only chart for **Temporal Server**. Wraps the upstream
**`temporal/temporal`** Helm chart.

```bash
helm repo add temporal https://go.temporal.io/helm-charts
helm install temporal temporal/temporal \
  --namespace platform-data --create-namespace \
  -f infrastructure/helm/charts/temporal/values.yaml
```

ADR-0009 § Part 4 "Temporal workflow":

> One Temporal cluster per bank, serves both versions. Workers
> (`onboarding-orchestrator`) are deployed in both blue and green
> namespaces and share the same task queues.

## Required pre-existing Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aibank-temporal-credentials
  namespace: platform-data
type: Opaque
stringData:
  password: <postgres-temporal-user-password>
```

The Postgres user `temporal` and the two databases (`temporal`, `temporal_visibility`)
must exist before installing the chart — schema-setup Job will create tables.

## TODO

- ServiceMonitor for Temporal metrics — deferred (kube-prometheus-stack required).
- TLS between server <-> workers via cert-manager Issuer — deferred.
- Multi-cluster replication — out of scope.
