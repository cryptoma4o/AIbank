# kafka (wrapper chart)

Values-only chart for **Apache Kafka in KRaft mode**. Wraps the upstream
**Bitnami `kafka`** chart.

```bash
helm install kafka bitnami/kafka \
  --namespace platform-data --create-namespace \
  -f infrastructure/helm/charts/kafka/values.yaml
```

Per ADR-0009 § Part 4 "Kafka": one cluster serves both `platform-blue` and
`platform-green` namespaces. Consumer groups are namespaced
(`bff-onboarding-blue`, `bff-onboarding-green`) so they advance offsets
independently during canary tests.

## TODO

- Schema Registry (Confluent / Karapace) — separate chart, deferred.
- TLS via cert-manager Issuer — deferred.
