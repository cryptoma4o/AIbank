// llm-gateway k6 benchmark (mock mode).
//
// Цель: измерить overhead самого gateway (роутинг + usage tracking + headers)
// БЕЗ влияния реальных LLM-провайдеров. Запускать с переменной окружения
// LLM_GATEWAY_FORCE_MOCK=1 на стороне сервиса (см. ai/llm-gateway/.env.example).
//
// SLA targets:
//   - p(95) < 500ms — mock-режим локальный и детерминированный, любая задержка
//     выше — это overhead роутинга/networking.
//   - p(99) < 1000ms — допустимый tail для cold-start lazy backend lookup.
//
// Сценарий:
//   - ramp-up до 50 VUs за 30s
//   - steady на 50 VUs в течение 60s
//
// Все запросы:
//   - POST /v1/chat/completions
//   - role:ru-chat (внутри gateway резолвится в mock backend)
//   - X-Tenant-Id ротируется по {bank-alpha, bank-beta, bank-gamma}
//   - 3 шаблона content для variability
//   - Ответ обязан содержать поле aibank_gateway (метаданные роутинга)
//
// Запуск:
//   LLM_GATEWAY_FORCE_MOCK=1 docker compose up -d llm-gateway
//   k6 run tools/benchmarks/k6/llm-gateway-bench.js

import http from 'k6/http';
import { check, group } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8100';

const TENANT_POOL = ['bench-alpha', 'bench-beta', 'bench-gamma'];

// 3 шаблона контента — короткий, средний, длинный — чтобы measure varied
// payload sizes. В mock-режиме длина не влияет на latency сильно, но бьёт по
// network/serialization stack.
const CONTENT_TEMPLATES = [
  'Привет, проверка benchmark теста.',
  'Расскажи про процедуру открытия расчётного счёта для ИП в дистанционном формате.',
  'Дай развёрнутый ответ: какие документы нужны для открытия счёта ООО на ОСНО, ' +
    'какие лимиты по обороту, как происходит верификация УБО, какие риски комплаенса ' +
    'учитываются при автоматическом скоринге заявки в формате 115-ФЗ?',
];

const gatewayOverhead = new Trend('llm_gateway_overhead_ms', true);
const missingMetadata = new Counter('llm_missing_metadata');
const fallbacks = new Counter('llm_used_fallback');

export const options = {
  stages: [
    { duration: '30s', target: 50 }, // ramp-up
    { duration: '60s', target: 50 }, // steady
    { duration: '15s', target: 0 },  // ramp-down
  ],
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
    http_req_failed: ['rate<0.01'],
    'llm_missing_metadata': ['count<10'],
  },
  tags: {
    service: 'llm-gateway',
    mode: 'mock',
    test_type: 'baseline',
  },
};

function pickRandom(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

export default function () {
  const tenantId = pickRandom(TENANT_POOL);
  const content = pickRandom(CONTENT_TEMPLATES);

  const payload = {
    model: 'role:ru-chat',
    messages: [
      { role: 'user', content: content },
    ],
    temperature: 0.7,
    max_tokens: 256,
    stream: false,
  };

  group('POST /v1/chat/completions (mock)', () => {
    const res = http.post(`${BASE_URL}/v1/chat/completions`, JSON.stringify(payload), {
      headers: {
        'Content-Type': 'application/json',
        'X-Tenant-Id': tenantId,
      },
      tags: { op: 'chat_completions', tenant: tenantId },
    });
    gatewayOverhead.add(res.timings.duration);

    const checks = check(res, {
      'status 200': (r) => r.status === 200,
      'has aibank_gateway metadata': (r) => {
        try { return !!r.json('aibank_gateway'); } catch (_) { return false; }
      },
      'tenant_id matches': (r) => {
        try { return r.json('aibank_gateway.tenant_id') === tenantId; } catch (_) { return false; }
      },
      'has choices array': (r) => {
        try { return Array.isArray(r.json('choices')); } catch (_) { return false; }
      },
    });

    if (!checks) {
      try {
        if (!res.json('aibank_gateway')) {
          missingMetadata.add(1);
        }
        if (res.json('aibank_gateway.used_fallback') === true) {
          fallbacks.add(1);
        }
      } catch (_) { /* response not JSON — already counted as failed */ }
    }
  });
}

export function handleSummary(data) {
  return {
    stdout: JSON.stringify(data, null, 2),
  };
}
