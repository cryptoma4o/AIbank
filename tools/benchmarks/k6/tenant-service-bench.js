// tenant-service k6 benchmark.
//
// Цель: получить baseline по tenant-service (CRUD + list).
//
// SLA targets из docs/product-vision.md § 6:
//   - Capacity: 5-50K заявок в год → ~60-600 RPM peak (~1-10 RPS sustained).
//   - С учётом запаса по нагрузке: planning headroom ~20x → ~200 RPS.
//   - p(95) latency для метаданных тенанта < 100ms (read-heavy).
//
// Сценарий:
//   - ramp-up до 100 VUs за 30s
//   - steady на 100 VUs в течение 60s
//   - ramp-down 30s
//
// Распределение операций:
//   - 70% GET  /v1/tenants/{id}     (read-heavy: чаще всего читаем по id)
//   - 20% GET  /v1/tenants          (list)
//   - 10% POST /v1/tenants          (create — random tenant_id `bench${VU}_${ITER}`)
//
// Запуск:
//   k6 run tools/benchmarks/k6/tenant-service-bench.js
//   BASE_URL=http://staging.aibank.local:8080 k6 run tools/benchmarks/k6/tenant-service-bench.js

import http from 'k6/http';
import { check, group } from 'k6';
import { Counter, Trend } from 'k6/metrics';

// ─── Конфигурация ────────────────────────────────────────────────────────
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

// Идемпотентные тестовые тенанты, засеянные scripts/seed.sh.
const SEED_TENANTS = ['bench-alpha', 'bench-beta', 'bench-gamma', 'bench-delta', 'bench-epsilon'];

// Кастомные метрики.
const createSuccess = new Counter('tenant_create_success');
const createDuplicate = new Counter('tenant_create_duplicate');
const getByIdLatency = new Trend('tenant_get_by_id_ms', true);

export const options = {
  stages: [
    { duration: '30s', target: 100 }, // ramp-up
    { duration: '60s', target: 100 }, // steady
    { duration: '30s', target: 0 },   // ramp-down
  ],
  thresholds: {
    http_req_duration: ['p(95)<100'],     // p95 < 100ms (target из vision § 6)
    http_req_failed: ['rate<0.01'],       // < 1% ошибок
    'tenant_get_by_id_ms': ['p(95)<80'],  // hot path: чтение по id ещё быстрее
  },
  // Tags для удобной фильтрации в результатах.
  tags: {
    service: 'tenant-service',
    test_type: 'baseline',
  },
};

function pickRandom(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

export default function () {
  const dice = Math.random();

  if (dice < 0.70) {
    // 70% — GET /v1/tenants/{id}
    group('GET /v1/tenants/{id}', () => {
      const id = pickRandom(SEED_TENANTS);
      const res = http.get(`${BASE_URL}/v1/tenants/${id}`, {
        tags: { op: 'get_by_id' },
      });
      getByIdLatency.add(res.timings.duration);
      check(res, {
        'status 200 or 404': (r) => r.status === 200 || r.status === 404,
        'has body': (r) => r.body && r.body.length > 0,
      });
    });
  } else if (dice < 0.90) {
    // 20% — GET /v1/tenants (list)
    group('GET /v1/tenants', () => {
      const res = http.get(`${BASE_URL}/v1/tenants`, {
        tags: { op: 'list' },
      });
      check(res, {
        'status 200': (r) => r.status === 200,
        'items array present': (r) => r.json('items') !== undefined || r.json('count') !== undefined,
      });
    });
  } else {
    // 10% — POST /v1/tenants (create)
    group('POST /v1/tenants', () => {
      const tenantId = `bench${__VU}_${__ITER}`;
      const payload = JSON.stringify({
        id: tenantId,
        name: `Bench tenant ${tenantId}`,
        bik: '044525593',
        inn: '7728168971',
        deployment_mode: 'saas',
      });
      const res = http.post(`${BASE_URL}/v1/tenants`, payload, {
        headers: { 'Content-Type': 'application/json' },
        tags: { op: 'create' },
      });
      // 201 — создано; 409 — re-run после идемпотентного теста.
      const ok = check(res, {
        'status 201 or 409': (r) => r.status === 201 || r.status === 409,
      });
      if (res.status === 201) {
        createSuccess.add(1);
      } else if (res.status === 409 || res.status === 500) {
        // 500 встречается при duplicate-провижне — считаем как duplicate.
        createDuplicate.add(1);
      }
      if (!ok) {
        console.error(`unexpected status ${res.status} for tenant ${tenantId}: ${res.body}`);
      }
    });
  }
}

export function handleSummary(data) {
  return {
    stdout: JSON.stringify(data, null, 2),
  };
}
