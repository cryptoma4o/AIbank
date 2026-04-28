// fixtures/mocks.ts — Playwright route interception for backend mocks.
//
// We deliberately avoid running msw inside the browser (it would require
// the next-mswjs adapter and complicate the SSR boundary).  Instead we
// use Playwright's built-in network interception, which is the
// idiomatic approach for e2e tests against a real Next.js dev server.
//
// MSW is still listed as a devDependency for component-test (jsdom) usage
// in future iterations, per the task brief.

import type { Page, Route } from "@playwright/test";
import { makeFakeJwt } from "./auth";

export interface MockApplication {
  id: string;
  tenantId: string;
  state: string;
  legalEntityType: "IP" | "LLC" | "JSC";
  channel: string;
  productCodes: string[];
  createdAt: string;
  updatedAt: string;
}

export const SAMPLE_APPLICATIONS: MockApplication[] = [
  {
    id: "11111111-1111-1111-1111-111111111111",
    tenantId: "bank_alpha",
    state: "collecting_documents",
    legalEntityType: "LLC",
    channel: "web",
    productCodes: ["current_account_rub", "acquiring"],
    createdAt: "2026-04-01T08:30:00Z",
    updatedAt: "2026-04-02T11:15:00Z",
  },
  {
    id: "22222222-2222-2222-2222-222222222222",
    tenantId: "bank_alpha",
    state: "auto_approved",
    legalEntityType: "IP",
    channel: "web",
    productCodes: ["current_account_rub"],
    createdAt: "2026-03-18T13:00:00Z",
    updatedAt: "2026-03-19T09:00:00Z",
  },
];

export const SAMPLE_APPLICATION_DETAIL = {
  ...SAMPLE_APPLICATIONS[0],
  applicant: {
    id: "applicant-001",
    fullName: "Иванов Иван Иванович",
    inn: "7701234567",
    phone: "+79001234567",
    email: "ivanov@example.com",
  },
  documents: [
    {
      id: "doc-1",
      type: "PASSPORT",
      applicationId: SAMPLE_APPLICATIONS[0].id,
      filename: "passport.pdf",
      state: "uploaded",
      uploadedAt: "2026-04-01T09:00:00Z",
    },
  ],
  riskAssessment: {
    id: "risk-1",
    score: 0.42,
    category: "LOW",
    recommendation: "Можно одобрить автоматически.",
    computedAt: "2026-04-02T10:00:00Z",
  },
};

export const NEW_APPLICATION_ID = "33333333-3333-3333-3333-333333333333";

interface InstallMocksOptions {
  /** Override identity URL prefix.  Defaults to the `__mock__` path used by playwright.config. */
  identityPath?: string;
  /** Override BFF GraphQL URL. */
  graphqlPath?: string;
  /** Force /v1/auth/login to fail with given status (default = success). */
  loginStatus?: number;
}

/**
 * Installs route interceptors for:
 *   POST  {identity}/v1/auth/login    → { access_token, refresh_token }
 *   GET   {identity}/v1/me            → applicant profile
 *   POST  {graphql}                  → multiplexes by `operationName`
 */
export async function installMocks(
  page: Page,
  opts: InstallMocksOptions = {}
): Promise<void> {
  const identityPath = opts.identityPath ?? "**/__mock__/identity/**";
  const graphqlPath = opts.graphqlPath ?? "**/__mock__/graphql";

  // ---- identity-service: /v1/auth/login ----
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
          access_token: makeFakeJwt({
            tenant_id: body.tenant_id ?? "bank_alpha",
            email: body.email,
          }),
          refresh_token: makeFakeJwt({
            tenant_id: body.tenant_id ?? "bank_alpha",
            email: body.email,
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
          id: "applicant-001",
          tenant_id: "bank_alpha",
          email: "applicant@example.com",
          role: "bank.applicant",
          is_active: true,
        }),
      });
    }
    return route.fallback();
  });

  // ---- bff-onboarding: GraphQL ----
  await page.route(graphqlPath, async (route: Route) => {
    if (route.request().method() !== "POST") return route.fallback();
    let body: { operationName?: string; variables?: Record<string, unknown> } = {};
    try {
      body = JSON.parse(route.request().postData() ?? "{}");
    } catch {
      /* ignore */
    }
    const op = body.operationName;

    if (op === "MyApplications") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { myApplications: SAMPLE_APPLICATIONS } }),
      });
    }
    if (op === "Application") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { application: SAMPLE_APPLICATION_DETAIL } }),
      });
    }
    if (op === "SubmitApplication") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            submitApplication: {
              id: NEW_APPLICATION_ID,
              state: "draft",
              legalEntityType: "LLC",
              channel: "web",
              productCodes: ["current_account_rub"],
              createdAt: "2026-04-26T10:00:00Z",
            },
          },
        }),
      });
    }
    if (op === "Me") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            me: {
              userId: "applicant-001",
              tenantId: "bank_alpha",
              role: "bank.applicant",
              email: "applicant@example.com",
            },
          },
        }),
      });
    }
    // Unknown operation — return empty data, never 500.
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ data: {} }),
    });
  });
}
