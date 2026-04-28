"use client";

// /tenant — read-only просмотр конфигурации тенанта.
//
// Запись tenant.yaml идёт через GitOps (см. ADR-0003 § 5: write-path —
// PR в репозиторий конфигов, не GraphQL).  Здесь — только просмотр со
// табами по основным секциям.  YAML парсим лениво и устойчиво к ошибкам;
// если секции нет — показываем заглушку.

import { useMemo, useState } from "react";
import { useQuery } from "@apollo/client";
import { format } from "date-fns";
import { ru } from "date-fns/locale";

import { AuthGuard } from "@/components/AuthGuard";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { QUERY_TENANT, QUERY_TENANT_CONFIG } from "@/lib/graphql-operations";
import { TENANT_CONFIG_ROLES } from "@/lib/roles";
import type { Tenant, TenantConfig } from "@/types";

interface ConfigData {
  tenantConfig: TenantConfig | null;
}

interface TenantData {
  tenant: Tenant | null;
}

type TabKey =
  | "branding"
  | "workflows"
  | "risk-policy"
  | "integrations"
  | "ai"
  | "sla"
  | "raw";

const TABS: { key: TabKey; label: string }[] = [
  { key: "branding", label: "Брендинг" },
  { key: "workflows", label: "Воркфлоу" },
  { key: "risk-policy", label: "Риск-политика" },
  { key: "integrations", label: "Интеграции" },
  { key: "ai", label: "AI-агенты" },
  { key: "sla", label: "SLA" },
  { key: "raw", label: "raw YAML" },
];

const SECTION_KEYS: Record<TabKey, string[]> = {
  branding: ["branding", "ui"],
  workflows: ["workflows", "scenarios", "states"],
  "risk-policy": ["risk_policy", "risk", "scoring"],
  integrations: ["integrations", "abs", "esia", "egrul"],
  ai: ["ai", "agents", "llm"],
  sla: ["sla"],
  raw: [],
};

/** Очень упрощённый «грeп» по YAML — выделяет блоки, начинающиеся с
 *  одного из ключей (top-level: `<key>:` без отступа).  Этого достаточно,
 *  чтобы дать оператору представление о размере секции; полноценный
 *  YAML-парсер мы не подключаем (избыточно для read-only превью). */
function extractSection(yaml: string, keys: string[]): string {
  const lines = yaml.split(/\r?\n/);
  const blocks: string[] = [];
  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i];
    const m = line.match(/^([a-zA-Z_][a-zA-Z0-9_]*)\s*:/);
    if (!m) continue;
    if (!keys.includes(m[1])) continue;
    const block: string[] = [line];
    let j = i + 1;
    while (j < lines.length) {
      const nl = lines[j];
      if (/^\S/.test(nl) && /^[a-zA-Z_]/.test(nl)) break;
      block.push(nl);
      j += 1;
    }
    blocks.push(block.join("\n"));
  }
  return blocks.join("\n\n").trim();
}

function formatTs(v: string): string {
  try {
    return format(new Date(v), "dd MMM yyyy, HH:mm", { locale: ru });
  } catch {
    return v;
  }
}

function TenantHeader({ tenant }: { tenant: Tenant | null }) {
  if (!tenant) {
    return (
      <div className="rounded-md border border-gray-200 bg-white p-4 text-sm text-gray-500">
        Информация о тенанте недоступна (возможно, вы platform.admin без
        привязки к тенанту).
      </div>
    );
  }
  return (
    <header className="rounded-md border border-gray-200 bg-white p-4">
      <div className="flex flex-wrap items-baseline justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">{tenant.name}</h1>
          <p className="mt-1 text-sm text-gray-500">
            ID <span className="font-mono">{tenant.id}</span> · ИНН{" "}
            <span className="font-mono">{tenant.inn}</span> · БИК{" "}
            <span className="font-mono">{tenant.bik}</span>
          </p>
        </div>
        <div className="text-sm">
          <span className="rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-700">
            {tenant.status}
          </span>
          <span className="ml-2 text-xs text-gray-500">
            режим: {tenant.deploymentMode}
          </span>
        </div>
      </div>
    </header>
  );
}

function TenantContent() {
  const tenantQuery = useQuery<TenantData>(QUERY_TENANT);
  const cfgQuery = useQuery<ConfigData>(QUERY_TENANT_CONFIG);
  const [tab, setTab] = useState<TabKey>("branding");

  const yaml = cfgQuery.data?.tenantConfig?.rawConfig ?? "";
  const sectionText = useMemo(() => {
    if (tab === "raw") return yaml;
    return extractSection(yaml, SECTION_KEYS[tab]);
  }, [yaml, tab]);

  return (
    <div className="space-y-4">
      <TenantHeader tenant={tenantQuery.data?.tenant ?? null} />

      {tenantQuery.error ? <ErrorBanner error={tenantQuery.error} /> : null}
      {cfgQuery.error ? <ErrorBanner error={cfgQuery.error} /> : null}

      <section className="rounded-md border border-gray-200 bg-white">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-4 py-3">
          <div>
            <h2 className="text-sm font-semibold text-gray-900">
              Конфигурация тенанта
            </h2>
            {cfgQuery.data?.tenantConfig ? (
              <p className="text-xs text-gray-500">
                схема {cfgQuery.data.tenantConfig.schemaVersion} · обновлена{" "}
                {formatTs(cfgQuery.data.tenantConfig.updatedAt)}
              </p>
            ) : (
              <p className="text-xs text-gray-500">read-only · правки через GitOps</p>
            )}
          </div>
        </div>

        <nav className="flex flex-wrap gap-1 border-b border-gray-200 px-4 py-2">
          {TABS.map((t) => (
            <button
              key={t.key}
              type="button"
              onClick={() => setTab(t.key)}
              className={`rounded-md px-2.5 py-1 text-xs font-medium ${
                tab === t.key
                  ? "bg-primary-50 text-primary"
                  : "text-gray-600 hover:bg-gray-50"
              }`}
            >
              {t.label}
            </button>
          ))}
        </nav>

        <div className="px-4 py-4">
          {cfgQuery.loading && !cfgQuery.data ? (
            <LoadingSpinner label="Загружаем конфиг…" />
          ) : !cfgQuery.data?.tenantConfig ? (
            <p className="text-sm text-gray-500">
              Конфигурация ещё не загружена в tenant-service.
            </p>
          ) : sectionText ? (
            <pre className="max-h-[60vh] overflow-auto rounded-md border border-gray-100 bg-gray-50 p-3 font-mono text-xs leading-relaxed text-gray-800">
              {sectionText}
            </pre>
          ) : (
            <p className="text-sm text-gray-500">
              В YAML нет секций {SECTION_KEYS[tab].map((k) => `"${k}"`).join(" / ")}
              . Используйте вкладку «raw YAML», чтобы увидеть полный конфиг.
            </p>
          )}
        </div>
      </section>

      <p className="text-xs text-gray-500">
        Изменение конфигурации — через pull-request в репозиторий
        configs/tenants/&lt;id&gt;/tenant.yaml; изменения автоматически
        применяются после ревью и merge.
      </p>
    </div>
  );
}

export default function TenantPage() {
  return (
    <AuthGuard allowRoles={TENANT_CONFIG_ROLES}>
      <TenantContent />
    </AuthGuard>
  );
}
