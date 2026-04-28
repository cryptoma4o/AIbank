// bff-onboarding (GraphQL) k6 benchmark.
//
// BFF — composite API: каждый резолвер делает fan-out в downstream-сервисы
// (tenant, identity, document, orchestrator, risk-engine), поэтому latency BFF
// = max(downstream_latency) + gateway_overhead.
//
// SLA targets:
//   - p(95) < 200ms — agg по upstream-сервисам.
//   - p(99) < 500ms — допускает редкие cold queries.
//
// Сценарий:
//   - ramp-up до 100 VUs за 30s
//   - steady на 100 VUs в течение 60s
//
// Распределение операций:
//   - 60% Query application(id)        — точечные запросы (по pre-seeded id)
//   - 30% Query myApplications         — list для текущего пользователя
//   - 10% Mutation submitApplication   — создаёт заявку
//
// Авторизация: mock JWT. Реальный JWT-issuance — в identity-service.
//
// Запуск:
//   BASE_URL=http://localhost:4000 k6 run tools/benchmarks/k6/bff-onboarding-bench.js

import http from 'k6/http';
import { check, group } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:4000';
const GRAPHQL_PATH = __ENV.GRAPHQL_PATH || '/graphql';
// Mock JWT — bff в test-режиме принимает любой Bearer-токен и парсит claims из
// X-Mock-Claims (см. apps/web-onboarding tests). На проде — реальный JWKS.
const MOCK_JWT = __ENV.MOCK_JWT || 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.' +
  'eyJzdWIiOiJiZW5jaC11c2VyIiwidGVuYW50X2lkIjoiYmVuY2gtYWxwaGEiLCJyb2xlIjoiYXBwbGljYW50In0.' +
  'mock-signature-not-validated-in-test-mode';

// Pre-seeded application ids — производятся scripts/seed.sh.
// На дев-стенде заявки создаются через mutation submitApplication.
// В benchmark mode мы либо читаем их из .seed-state.json, либо используем
// предсказуемые id вида "app-bench-alpha-{1..50}".
const APPLICATION_IDS = [];
for (let i = 1; i <= 50; i++) {
  APPLICATION_IDS.push(`app-bench-alpha-${i}`);
}

// ─── GraphQL queries ─────────────────────────────────────────────────────
const QUERY_APPLICATION = `
query GetApplication($id: ID!) {
  application(id: $id) {
    id
    tenantId
    state
    legalEntityType
    channel
    productCodes
    createdAt
    updatedAt
    applicant { id fullName inn }
    documents { id type filename state }
    riskAssessment { score category recommendation }
    decision { decision reasoning }
  }
}`;

const QUERY_MY_APPLICATIONS = `
query MyApplications {
  myApplications {
    id
    state
    legalEntityType
    createdAt
  }
}`;

const MUTATION_SUBMIT_APPLICATION = `
mutation SubmitApplication($input: SubmitApplicationInput!) {
  submitApplication(input: $input) {
    id
    state
    createdAt
  }
}`;

const queryLatency = new Trend('bff_query_application_ms', true);
const listLatency = new Trend('bff_my_applications_ms', true);
const mutationLatency = new Trend('bff_submit_mutation_ms', true);
const graphqlErrors = new Counter('bff_graphql_errors');

export const options = {
  stages: [
    { duration: '30s', target: 100 }, // ramp-up
    { duration: '60s', target: 100 }, // steady
    { duration: '15s', target: 0 },   // ramp-down
  ],
  thresholds: {
    http_req_duration: ['p(95)<200', 'p(99)<500'],
    http_req_failed: ['rate<0.01'],
    'bff_query_application_ms': ['p(95)<200'],
    'bff_my_applications_ms': ['p(95)<300'], // list чуть медленнее
    'bff_graphql_errors': ['count<50'],
  },
  tags: {
    service: 'bff-onboarding',
    test_type: 'baseline',
  },
};

function pickRandom(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

function gqlRequest(query, variables, opTag) {
  const body = JSON.stringify({ query, variables: variables || {} });
  return http.post(`${BASE_URL}${GRAPHQL_PATH}`, body, {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${MOCK_JWT}`,
    },
    tags: { op: opTag },
  });
}

function checkGraphQLOk(res, opName) {
  const ok = check(res, {
    'status 200': (r) => r.status === 200,
    'no graphql errors': (r) => {
      try {
        const errors = r.json('errors');
        return !errors || errors.length === 0;
      } catch (_) {
        return false;
      }
    },
  });
  if (!ok) {
    graphqlErrors.add(1);
  }
  return ok;
}

export default function () {
  const dice = Math.random();

  if (dice < 0.60) {
    // 60% — Query application(id)
    group('Query application(id)', () => {
      const id = pickRandom(APPLICATION_IDS);
      const res = gqlRequest(QUERY_APPLICATION, { id }, 'query_application');
      queryLatency.add(res.timings.duration);
      checkGraphQLOk(res, 'application');
    });
  } else if (dice < 0.90) {
    // 30% — Query myApplications
    group('Query myApplications', () => {
      const res = gqlRequest(QUERY_MY_APPLICATIONS, {}, 'query_my_applications');
      listLatency.add(res.timings.duration);
      checkGraphQLOk(res, 'myApplications');
    });
  } else {
    // 10% — Mutation submitApplication
    group('Mutation submitApplication', () => {
      const input = {
        legalEntityType: 'OOO',
        channel: 'web',
        productCodes: ['settlement_account'],
      };
      const res = gqlRequest(MUTATION_SUBMIT_APPLICATION, { input }, 'mutation_submit');
      mutationLatency.add(res.timings.duration);
      checkGraphQLOk(res, 'submitApplication');
    });
  }
}

export function handleSummary(data) {
  return {
    stdout: JSON.stringify(data, null, 2),
  };
}
