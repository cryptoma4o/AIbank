// audit-service k6 benchmark.
//
// Цель: проверить, что append-only журнал выдерживает high-traffic ingestion.
// Audit события генерируются ВСЕМИ сервисами при изменении состояния, поэтому
// нагрузка на audit-service в 10-50x выше типового CRUD-сервиса.
//
// SLA targets из docs/product-vision.md § 6:
//   - 5-50K заявок/год × ~50 событий на заявку = 250K-2.5M событий/год.
//   - Peak ~1000 событий/мин при 50K заявок → 17 RPS sustained, 100+ RPS peak.
//   - С запасом 10x: 1000+ RPS под нагрузкой стресс-теста.
//   - p(95) ingestion latency < 50ms (БД INSERT с hash-цепочкой).
//   - p(99) < 200ms (включая GC pauses, hash-recompute).
//
// Сценарий:
//   - ramp-up до 500 VUs за 60s
//   - steady на 500 VUs в течение 120s
//
// Распределение операций:
//   - 90% POST /v1/events           (append, payload ~500 bytes)
//   - 10% GET  /v1/events?tenant_id (list, limit 50)
//
// Запуск:
//   k6 run tools/benchmarks/k6/audit-service-bench.js

import http from 'k6/http';
import { check, group } from 'k6';
import { Counter, Trend } from 'k6/metrics';

// ─── Конфигурация ────────────────────────────────────────────────────────
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8081';

// Фиксированный пул тенантов — соответствует scripts/seed.sh.
const TENANT_POOL = ['bench-alpha', 'bench-beta', 'bench-gamma', 'bench-delta', 'bench-epsilon'];
const EVENT_TYPES = [
  'application.created',
  'application.updated',
  'document.uploaded',
  'document.parsed',
  'risk.assessed',
  'decision.made',
  'workflow.signal',
];
const ENTITY_TYPES = ['application', 'document', 'person', 'risk_assessment'];
const ACTOR_TYPES = ['user', 'system', 'ai_agent'];

// ~500 байт payload — типовой размер audit-события (ID + meta + diff).
const SAMPLE_PAYLOAD = {
  field_changes: {
    state: { old: 'draft', new: 'identifying' },
    updated_by: 'workflow:abc-123',
  },
  context: {
    workflow_id: 'wf-temporal-12345',
    activity_id: 'act-67890',
    correlation_id: 'corr-aaaaaaaaaaaaaaaa',
  },
  metadata: {
    source: 'orchestrator',
    version: '1.2.3',
    region: 'ru-central-1',
    pod: 'orchestrator-7d8f9c-x2k4l',
    extra_tags: ['high_priority', 'compliance_required', 'audit_critical'],
  },
};

const ingestLatency = new Trend('audit_ingest_ms', true);
const listLatency = new Trend('audit_list_ms', true);
const ingestSuccess = new Counter('audit_ingest_success');
const ingestFailures = new Counter('audit_ingest_failures');

export const options = {
  stages: [
    { duration: '60s',  target: 500 }, // ramp-up
    { duration: '120s', target: 500 }, // steady
    { duration: '30s',  target: 0 },   // ramp-down
  ],
  thresholds: {
    http_req_duration: ['p(95)<50', 'p(99)<200'],
    http_req_failed: ['rate<0.005'],     // < 0.5% ошибок
    'audit_ingest_ms': ['p(95)<50', 'p(99)<200'],
    'audit_ingest_failures': ['count<100'],
  },
  tags: {
    service: 'audit-service',
    test_type: 'high_load',
  },
};

function pickRandom(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

export default function () {
  const dice = Math.random();

  if (dice < 0.90) {
    // 90% — POST /v1/events
    group('POST /v1/events', () => {
      const tenantId = pickRandom(TENANT_POOL);
      const event = {
        tenant_id: tenantId,
        entity_type: pickRandom(ENTITY_TYPES),
        entity_id: `ent-${__VU}-${__ITER}`,
        event_type: pickRandom(EVENT_TYPES),
        actor_id: `actor-${__VU}`,
        actor_type: pickRandom(ACTOR_TYPES),
        payload: SAMPLE_PAYLOAD,
      };
      const res = http.post(`${BASE_URL}/v1/events`, JSON.stringify(event), {
        headers: { 'Content-Type': 'application/json' },
        tags: { op: 'ingest' },
      });
      ingestLatency.add(res.timings.duration);
      const ok = check(res, {
        'status 201': (r) => r.status === 201,
        'has hash': (r) => {
          try { return !!r.json('hash'); } catch (_) { return false; }
        },
      });
      if (ok) {
        ingestSuccess.add(1);
      } else {
        ingestFailures.add(1);
      }
    });
  } else {
    // 10% — GET /v1/events?tenant_id=X&limit=50
    group('GET /v1/events', () => {
      const tenantId = pickRandom(TENANT_POOL);
      const res = http.get(`${BASE_URL}/v1/events?tenant_id=${tenantId}&limit=50`, {
        tags: { op: 'list' },
      });
      listLatency.add(res.timings.duration);
      check(res, {
        'status 200': (r) => r.status === 200,
        'items array present': (r) => {
          try { return Array.isArray(r.json('items')); } catch (_) { return false; }
        },
      });
    });
  }
}

export function handleSummary(data) {
  return {
    stdout: JSON.stringify(data, null, 2),
  };
}
