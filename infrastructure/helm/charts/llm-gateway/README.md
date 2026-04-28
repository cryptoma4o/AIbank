# llm-gateway

Helm chart for the AIbank **LLM Gateway** (FastAPI / Python).

The gateway is the single egress point for all AI agents (`agent-document-intake`,
`agent-reconciliation`, `agent-ubo-tracing`, `agent-conversational`,
`agent-risk-scoring`, `agent-compliance-assistant`). It enforces auth, rate
limits, prompt logging, and routing to one of:

- Mock backend (default in MVP, `LLM_GATEWAY_FORCE_MOCK=1`)
- vLLM running on GPU nodes (production self-hosted)
- External OpenAI-compatible endpoint

## Differences from `aibank-service`

| Aspect | `aibank-service` | `llm-gateway` |
|---|---|---|
| Port | `8080` (configurable) | **`8100`** |
| Health path | `/health` + `/ready` | **`/healthz`** for both |
| Startup probe | optional | **enabled by default**, 60-attempt window for vLLM cold start |
| GPU support | no | optional, OFF in MVP |
| Persistent model cache | no | optional PVC at `/models`, OFF in MVP |
| Resource defaults | 200m / 256Mi | 250m / 512Mi (Python overhead) |

## Usage

```bash
helm install llm-gateway ./charts/llm-gateway \
  --namespace platform-ai --create-namespace \
  --set image.tag=0.1.0
```

## Production overlay (vLLM on GPU)

```yaml
# values-prod.yaml
gpu:
  enabled: true
  count: 1
  nodeSelector:
    cloud.google.com/gke-accelerator: nvidia-tesla-a100
  tolerations:
    - key: nvidia.com/gpu
      operator: Exists
      effect: NoSchedule

modelCache:
  enabled: true
  size: 200Gi
  storageClass: fast-ssd

env:
  - name: LLM_GATEWAY_FORCE_MOCK
    value: "0"
  - name: VLLM_BASE_URL
    value: http://vllm.platform-ai.svc.cluster.local:8000
  - name: LOG_REQUESTS
    value: "true"

resources:
  limits:
    cpu: 4
    memory: 16Gi
  requests:
    cpu: 1
    memory: 4Gi
```

## Validating

```bash
helm lint     ./charts/llm-gateway
helm template ./charts/llm-gateway
```

## TODO

- Sidecar for Vault Agent injection of provider API keys (deferred).
- HorizontalPodAutoscaler with custom metric (token-throughput) — needs Prometheus Adapter.
- Deployment.spec.template.spec.priorityClassName once `aibank-critical` PriorityClass is defined.
