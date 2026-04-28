// fixtures/mocks.ts — bff-admin mocks for web-admin smoke tests.

import type { Page, Route } from "@playwright/test";
import { makeOperatorJwt, type OperatorRole } from "./auth";

export interface MockAdminApplication {
  id: string;
  tenant_id: string;
  status: string;
  inn: string;
  ogrn: string;
}

export interface MockAuditEvent {
  id: string;
  event_type: string;
  actor_id: string;
  resource_id: string;
  occurred_at: string;
}

export const SAMPLE_ADMIN_APPLICATIONS: MockAdminApplication[] = [
  {
    id: "aaaaaaaa-1111-1111-1111-111111111111",
    tenant_id: "bank_alpha",
    status: "manual_review",
    inn: "7701234567",
    ogrn: "1027700123456",
  },
  {
    id: "bbbbbbbb-2222-2222-2222-222222222222",
    tenant_id: "bank_alpha",
    status: "auto_approved",
    inn: "7707654321",
    ogrn: "1027700654321",
  },
];

export const SAMPLE_AUDIT_EVENTS: MockAuditEvent[] = [
  {
    id: "evt-1",
    event_type: "application.submitted",
    actor_id: "applicant-001",
    resource_id: SAMPLE_ADMIN_APPLICATIONS[0].id,
    occurred_at: "2026-04-25T08:00:00Z",
  },
  {
    id: "evt-2",
    event_type: "document.uploaded",
    actor_id: "applicant-001",
    resource_id: SAMPLE_ADMIN_APPLICATIONS[0].id,
    occurred_at: "2026-04-25T08:30:00Z",
  },
  {
    id: "evt-3",
    event_type: "risk.assessed",
    actor_id: "agent-risk-scoring",
    resource_id: SAMPLE_ADMIN_APPLICATIONS[0].id,
    occurred_at: "2026-04-25T08:45:00Z",
  },
  {
    id: "evt-4",
    event_type: "application.approved",
    actor_id: "operator-007",
    resource_id: SAMPLE_ADMIN_APPLICATIONS[1].id,
    occurred_at: "2026-04-25T10:00:00Z",
  },
  {
    id: "evt-5",
    event_type: "tenant.policy.updated",
    actor_id: "admin-001",
    resource_id: "bank_alpha",
    occurred_at: "2026-04-26T07:15:00Z",
  },
];

interface InstallAdminMocksOptions {
  loginRole?: OperatorRole;
  loginStatus?: number;
  graphqlPath?: string;
  identityPath?: string;
}

/**
 * Installs route interceptors for bff-admin GraphQL + identity-service REST.
 *
 * GraphQL routes: applications list, audit events.
 * REST: /v1/auth/login (operator), /v1/me.
 *
 * Catch-all matches both `**\/graphql` and `**\/__mock__\/graphql` so the same
 * mocks work whether the app calls the rewritten URL or the canonical one.
 */
export async function installAdminMocks(
  page: Page,
  opts: InstallAdminMocksOptions = {}
): Promise<void> {
  const role = opts.loginRole ?? "bank.admin";
  const identityPath = opts.identityPath ?? "**/__mock__/identity/**";
  const graphqlPath = opts.graphqlPath ?? "**/__mock__/graphql";

  // ---- identity-service ----
  await page.route(identityPath, async (route: Route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith("/v1/auth/login")) {
      if (opts.loginStatus && opts.loginStatus >= 400) {
        return route.fulfill({
          status: opts.loginStatus,
          contentType: "application/json",
          body: JSON.stringify({
            error: { code: "invalid_credentials", message: "Bad creds" },
          }),
        });
      }
      const body = JSON.parse(route.request().postData() ?? "{}") as {
        tenant_id?: string;
        email?: string;
      };
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          access_token: makeOperatorJwt({
            tenant_id: body.tenant_id ?? "bank_alpha",
            email: body.email,
            role,
          }),
          refresh_token: makeOperatorJwt({
            tenant_id: body.tenant_id ?? "bank_alpha",
            email: body.email,
            role,
            type: "refresh",
          }),
          expires_in: 3600,
        }),
      });
    }
    if (url.pathname.endsWith("/v1/me")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          id: "operator-007",
          tenant_id: "bank_alpha",
          email: "operator@example.com",
          role,
          is_active: true,
        }),
      });
    }
    return route.fallback();
  });

  // ---- bff-admin GraphQL ----
  await page.route(graphqlPath, async (route: Route) => {
    if (route.request().method() !== "POST") return route.fallback();
    let body: { operationName?: string; query?: string; variables?: Record<string, unknown> } = {};
    try {
      body = JSON.parse(route.request().postData() ?? "{}");
    } catch {
      /* ignore */
    }
    const op = body.operationName ?? "";
    const query = body.query ?? "";

    // Match by operationName *or* by query substring (server components may not name ops).
    if (op === "GetApplications" || /applications\s*\(/.test(query)) {
      // Honour status filter from variables, if provided.
      const filterStatus = (body.variables?.status as string | undefined) ?? null;
      const items = filterStatus
        ? SAMPLE_ADMIN_APPLICATIONS.filter((a) => a.status === filterStatus)
        : SAMPLE_ADMIN_APPLICATIONS;
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { applications: items } }),
      });
    }
    if (op === "GetAuditEvents" || /auditEvents/.test(query)) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { auditEvents: SAMPLE_AUDIT_EVENTS } }),
      });
    }
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ data: {} }),
    });
  });
}
