<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-29 -->

# Server Sizing для AIbank

Конкретные требования к железу для разных режимов запуска платформы.
3 профиля: **минимум** (dev), **среднее** (staging/pilot), **максимум**
(production HA).

**Использовать**: при выборе VPS-провайдера, бюджетировании cluster'а,
обсуждении с DevOps/банком об on-prem deployment'е.

---

## TL;DR одной таблицей

| Профиль | Использование | RAM | vCPU | Disk | GPU |
|---------|---------------|-----|------|------|-----|
| **Минимум (Dev)** | 1 разработчик, smoke + demo | **8 GB** | 4 | 40 GB SSD | — (mock LLM) |
| **Среднее (Staging / Pilot)** | 1-3 банка, ~50 заявок/день | **24-32 GB** | 8-12 | 200 GB SSD | 1× A100 80GB (отдельно) |
| **Максимум (Production HA)** | 5-10 банков, ~5K заявок/день, fault-tolerant | **256-400 GB** (cluster) | 32-64 | 5-10 TB | 4× A100 / 2× H100 |

---

## 1. Минимум (Dev / Demo) — 8 GB RAM, 4 vCPU

### Use cases

- Один разработчик локально (или GitHub Codespace)
- `make up && make smoke && ./demo/run-demo.sh` — демо для встречи с банком
- CI runner для интеграционных тестов

### Что включено

- Все 21 Go-сервис в **single instance**
- 6 AI-агентов в **mock-backend режиме** (без реального LLM)
- 2 Next.js frontend в dev/SSR mode
- PostgreSQL 16 single-node (без replicas)
- Kafka KRaft single-node (replication factor 1)
- Redis 7 single instance
- Qdrant с минимальным RAG-корпусом
- MinIO single-node
- Temporal single-node

### Memory budget

| Компонент | RAM |
|-----------|-----|
| OS (Ubuntu 22.04) | 500 MB |
| PostgreSQL 16 (shared_buffers=128 MB) | 500 MB |
| Kafka KRaft (heap 1 GB) | 1.2 GB |
| Redis 7 | 200 MB |
| Qdrant (минимальный корпус) | 500 MB |
| MinIO | 200 MB |
| Temporal (PostgreSQL backend) | 500 MB |
| 21 Go-сервис idle (~80 MB каждый) | 1.7 GB |
| 6 AI agents Python idle (mock backend) | 1.5 GB |
| llm-gateway / rag-service / eval-harness | 750 MB |
| 2 frontend (Next.js dev mode) | 700 MB |
| **ИТОГО** | **~8 GB** |

### Где взять

| Опция | Цена | Lead-time |
|-------|------|-----------|
| **GitHub Codespaces** (4 vCPU / 8 GB) | бесплатно 60 ч/мес | мгновенно |
| **Selectel Cloud** (compact-1) | ~₽800/мес | 5 минут |
| **Yandex Cloud** (s-standard-2) | ~₽1 200/мес | 5 минут |
| **Hetzner Cloud** (CPX21) | €6/мес | 2 минуты |
| Личный Mac/PC с 16 GB | — | — |

### Disk

- 40 GB SSD (postgres data + docker images + logs)
- Из них: docker images ~10 GB, postgres data ~5 GB, logs ~2 GB

### Что НЕ работает

- Реальные LLM (нет GPU)
- HA / fault tolerance
- Production load (>10 одновременных заявок)

---

## 2. Среднее (Staging / Pilot) — 24-32 GB RAM, 8-12 vCPU

### Use cases

- Pilot с 1-3 банками-тенантами
- ~50-200 заявок в день
- Smoke на реальных данных (не synthetic)
- Внутренняя интеграционная среда

### Что включено

- Все 21 Go-сервис **single replica** (без HA, restart-on-fail)
- 6 AI-агентов с **реальным vLLM** (на отдельной GPU-машине)
- Frontend в production mode (`next build` + `next start`)
- PostgreSQL с реальным WAL и backup'ом
- Kafka KRaft (3 брокера для отказоустойчивости)
- Vault HA (3 узла, для управления секретами)
- Observability стек (VictoriaMetrics + Grafana + Loki) — отдельная нода

### Memory budget

| Компонент | RAM |
|-----------|-----|
| OS + system services | 1 GB |
| PostgreSQL (shared_buffers=2 GB, WAL, connections) | 3 GB |
| Kafka 3 брокера (1.5 GB heap каждый) | 5 GB |
| Redis | 500 MB |
| Qdrant (корпус 115-ФЗ + 375-П, ~10K docs) | 1 GB |
| MinIO | 500 MB |
| Temporal | 1 GB |
| Vault HA (3 × 512 MB) | 1.5 GB |
| 21 Go-сервис под нагрузкой (~200 MB каждый) | 4.2 GB |
| 6 AI agents Python production (~500 MB каждый) | 3 GB |
| llm-gateway / rag-service / eval-harness | 1.5 GB |
| 2 frontend (production SSR) | 1 GB |
| Buffer/cache + headroom | 2 GB |
| **ИТОГО** | **~24-26 GB** |

### Где взять

| Опция | Spec | Цена | Lead-time |
|-------|------|------|-----------|
| **Selectel Cloud** (general-2) | 8 vCPU / 32 GB / 200 GB | ~₽4 500/мес | 5 минут |
| **Yandex Cloud** (s-standard-8) | 8 vCPU / 32 GB / 200 GB | ~₽6 000/мес | 5 минут |
| **VK Cloud** | 8 vCPU / 32 GB / 200 GB | ~₽5 000/мес | 5 минут |
| **Hetzner Cloud** (CCX23) | 8 vCPU / 32 GB / 240 GB | €30/мес | 2 минуты |
| **On-prem 1U сервер** (для банка) | 8 cores / 32 GB / 240 GB SSD | ~₽150 000 (CapEx) | 2-4 нед |

### GPU (отдельная машина)

Для реального vLLM с моделями Gemma 4 / Qwen 3.5 / T-pro:

| Опция | VRAM | Цена |
|-------|------|------|
| **vast.ai 1× A100 80GB** (spot) | 80 GB | $0.5-1.2/час (~₽35K-90K/мес 24×7) |
| **Selectel GPU** (1× A100) | 80 GB | от ₽120K/мес |
| **Yandex Cloud** (1× A100) | 80 GB | от ₽180K/мес |
| **On-prem 1× H100** | 80 GB | ~₽4M (CapEx) |

**Рекомендация для пилота**: vast.ai spot — самый дешёвый старт, переключение на Selectel/Yandex для production.

### Disk

- 200 GB SSD основной
- 100 GB для PostgreSQL backups (raid или snapshots)
- 50 GB для Kafka log retention (3-7 дней)
- 30 GB для logs / metrics retention

---

## 3. Максимум (Production HA) — 256-400 GB RAM (cluster), 32-64 vCPU

### Use cases

- 5-10 банков-тенантов
- 1 000 - 5 000 заявок в день
- 99.9% uptime SLA
- Multi-region failover (для on-prem банков с DR)
- Полная регуляторная compliance (152-ФЗ, 187-ФЗ DR-drills)

### Архитектура

```
┌──────────────────────────────────────────────────────────────┐
│  k8s cluster (3-5 worker nodes × 32-128 GB RAM)              │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐             │
│  │ Worker-1   │  │ Worker-2   │  │ Worker-3   │  +N         │
│  │ 64 GB RAM  │  │ 64 GB RAM  │  │ 64 GB RAM  │             │
│  └────────────┘  └────────────┘  └────────────┘             │
│  ┌────────────┐  ┌────────────┐                              │
│  │ Master-1   │  │ Master-2   │  + odd quorum                │
│  │ 8 GB RAM   │  │ 8 GB RAM   │                              │
│  └────────────┘  └────────────┘                              │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│  Stateful pool (отдельно, dedicated nodes)                   │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │ Postgres    │  │ Kafka       │  │ MinIO       │          │
│  │ HA × 3      │  │ HA × 3      │  │ distrib × 4 │          │
│  │ 16 GB each  │  │ 8 GB each   │  │ 4 GB each   │          │
│  │ 500 GB SSD  │  │ 200 GB SSD  │  │ 5 TB SSD    │          │
│  └─────────────┘  └─────────────┘  └─────────────┘          │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│  GPU pool (отдельно, отдельный billing)                       │
│  ┌─────────────┐  ┌─────────────┐                            │
│  │ vLLM Gemma  │  │ vLLM Qwen   │  + T-pro on shared         │
│  │ A100 80GB   │  │ A100 80GB   │                            │
│  └─────────────┘  └─────────────┘                            │
└──────────────────────────────────────────────────────────────┘
```

### Memory budget

#### Stateful pool (~110 GB total)

| Компонент | Replicas | RAM каждая | Total |
|-----------|----------|------------|-------|
| PostgreSQL HA (1 master + 2 replica) | 3 | 16 GB | 48 GB |
| Kafka KRaft cluster | 3 | 8 GB | 24 GB |
| Redis Sentinel/Cluster | 6 | 4 GB | 24 GB |
| Qdrant cluster | 3 | 8 GB | 24 GB |
| MinIO distributed | 4 | 4 GB | 16 GB |
| Temporal HA | 3 | 4 GB | 12 GB |

#### Application pool (~80 GB total)

| Компонент | Replicas | RAM каждая | Total |
|-----------|----------|------------|-------|
| 21 backend Go-сервис × 3 (HA) = 63 pods | 63 | 500 MB | 32 GB |
| 6 AI-агентов × 3 = 18 pods | 18 | 1 GB | 18 GB |
| 2 frontend × 3 = 6 pods | 6 | 1 GB | 6 GB |
| llm-gateway × 3 | 3 | 1 GB | 3 GB |
| rag-service × 3 | 3 | 1.5 GB | 5 GB |

#### Platform pool (~50 GB total)

| Компонент | RAM |
|-----------|-----|
| Vault HA (3 узла) | 3 GB |
| Istio control plane (istiod + ingress) | 4 GB |
| VictoriaMetrics + vmagent + Grafana | 16 GB |
| Loki + promtail | 8 GB |
| Tempo (tracing) | 6 GB |
| ArgoCD / Flux (GitOps) | 2 GB |
| cert-manager + external-dns | 1 GB |
| K8s system (kube-system, ingress, CNI) | 8 GB |

#### Headroom

- 25% buffer для GC, spike, deploy: **+60 GB**

**ИТОГО: ~300 GB RAM cluster-wide**, спецификация уровня 3-5 worker нод × 64 GB + 2 master нод × 8 GB.

### CPU

- 32-64 vCPU per worker node (Intel Xeon Gold / AMD EPYC)
- Burst handling для AI-инференса в peak'и (UTC 9-18 МСК)

### Disk

| Компонент | Размер | Тип |
|-----------|--------|-----|
| PostgreSQL data (1 master + 2 replica × 500 GB) | 1.5 TB | NVMe SSD |
| PostgreSQL WAL archive | 500 GB | SSD |
| Kafka logs (retention 30 дней) | 600 GB | SSD |
| MinIO (документы клиентов, 5 банков × 1 TB) | 5 TB | SSD/HDD mix |
| Qdrant index | 100 GB | NVMe SSD |
| Backups (offsite, encrypted) | 5 TB | object storage |
| OS / docker images / logs | 200 GB | SSD |
| **Total** | **~13 TB** | |

### GPU

| Compute | Что обслуживает |
|---------|------------------|
| 2× A100 80GB (или 1× H100 80GB) | Gemma 4 (26B/31B) — document-vision + text-reasoning |
| 1× A100 80GB | Qwen 3.5 (27B + vl-32B) — reconciliation + UBO + RAG |
| 1× A100 80GB или shared | T-pro / Vikhr — ru-chat + быстрые ответы |
| 1× T4 / RTX A4000 | bge-m3 embedder + bge-reranker (TEI sidecar) |

**Минимум для production**: 4× A100 80GB ИЛИ 2× H100 80GB.

### Где взять

| Опция | Подходит для |
|-------|--------------|
| **Yandex Cloud k8s** (managed) | SaaS-tenants на Yandex (российский compliance) |
| **Selectel k8s** (managed) | альтернатива Yandex |
| **VK Cloud k8s** | для on-prem банков |
| **On-prem кластер** (3-5 серверов в банке) | банки с air-gapped requirements (КриптоПро HSM) |
| **Hetzner Dedicated Server** (Sweden DC) | дешёвый non-РФ pilot, не для production РФ-банков |

**Стоимость**:
- **Selectel managed k8s + storage + GPU**: ~₽300-500K/мес
- **Yandex Cloud managed**: ~₽400-700K/мес
- **On-prem CapEx (3 серверов + GPU): ~₽15-30M one-time**, OpEx ~₽100K/мес (электричество + cooling + maintenance)

---

## 4. Что НЕ влезет в малые серверы

Для понимания — почему текущий VPS 45.67.35.145 (1.9 GB RAM, 2 vCPU) не подходит:

| Компонент | Минимум RAM |
|-----------|-------------|
| PostgreSQL 16 + 21 Go-сервис idle | 2.5 GB |
| + 6 AI-агентов (mock LLM) | 4 GB |
| + 2 frontend SSR | 4.7 GB |
| + Kafka + Redis + MinIO + Temporal | 7 GB |
| + buffer для запросов | **8 GB минимум** |

**1.9 GB RAM** хватит на:
- 1 frontend (web-onboarding) в dev mode + nginx
- ИЛИ 3-4 backend Go-сервиса idle + Postgres
- НЕ хватит для полного `make up`

---

## 5. Recommended path

### Прямо сейчас (для тебя)

1. **GitHub Codespaces** — бесплатно, 8 GB RAM, для разработки/демо
2. Если нужна постоянная среда: **Selectel general-2** (8 vCPU / 32 GB / ~₽4500/мес)

### Для пилота с 1-м банком (через 2-3 мес)

1. **Selectel general-4** (16 vCPU / 64 GB / 500 GB SSD) — основная нода
2. **Selectel GPU** (1× A100 80GB) — vLLM, ~₽120K/мес
3. **Vault HA** на 3 малых nodes Selectel general-1 (по ₽1500/мес каждая)
4. **Total**: ~₽135-150K/мес

### Для production (через 6-9 мес, после первого LOI)

1. **Selectel managed k8s** (3-5 нод по 64 GB) — ~₽250K/мес
2. **2× A100 GPU** + 1× T4 (TEI) — ~₽250K/мес
3. **Stateful pool**: managed Postgres HA + Kafka HA + MinIO — ~₽100K/мес
4. **Total**: ~₽600K/мес OpEx

---

## 6. Чеклист перед заказом сервера

### Минимум (Dev / Demo)

- [ ] 8 GB RAM свободных (не "доступных", а реально не используемых)
- [ ] 4 vCPU
- [ ] 40 GB SSD
- [ ] Docker Engine 24+ или Docker Desktop
- [ ] Ubuntu 22.04 LTS (рекомендуется)
- [ ] Python 3.12, Go 1.22, Node 20

### Среднее (Pilot)

- [ ] 32 GB RAM
- [ ] 12 vCPU (с burst)
- [ ] 200 GB NVMe SSD
- [ ] Отдельная GPU-машина (1× A100 80GB или эквивалент)
- [ ] PostgreSQL 16 (managed или self-hosted)
- [ ] Vault HA (3 малых nodes)
- [ ] Резервное копирование (automated daily snapshots)
- [ ] Public IP + DNS A-record
- [ ] TLS-сертификат (Let's Encrypt или внутренний CA)

### Максимум (Production)

- [ ] k8s 1.30+ cluster (managed или self-hosted)
- [ ] 3-5 worker nodes × 64 GB
- [ ] Postgres HA (1 master + 2 replica), automatic failover
- [ ] Kafka KRaft 3+ брокера
- [ ] MinIO distributed mode (4+ nodes)
- [ ] Vault HA с auto-unseal через KMS (см. ADR-0013)
- [ ] Istio mesh (см. ADR-0014)
- [ ] 4× A100 GPU pool (или 2× H100)
- [ ] Observability стек (VictoriaMetrics + Loki + Tempo)
- [ ] DR site в другом регионе (для on-prem банков)
- [ ] Pen-test перед production rollout
- [ ] WAF (Cloudflare или собственный)
- [ ] PagerDuty / Slack alerting

---

## Связанные документы

- [docs/pilot-readiness.md](../pilot-readiness.md) — pre-pilot checklist
- [docs/adr/0013-vault-ha-topology.md](../adr/0013-vault-ha-topology.md) — Vault HA topology
- [docs/adr/0014-mtls-istio.md](../adr/0014-mtls-istio.md) — mTLS Istio
- [docs/devops-review-package.md](../devops-review-package.md) — для DevOps review
- [infrastructure/helm/charts/vault/README.md](../../infrastructure/helm/charts/vault/README.md) — Vault deployment
- [infrastructure/helm/charts/istio-mesh/README.md](../../infrastructure/helm/charts/istio-mesh/README.md) — Istio policies
