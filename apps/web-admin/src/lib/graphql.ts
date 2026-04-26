const BFF_URL = process.env.BFF_ADMIN_URL ?? "http://localhost:8095";

export async function gqlFetch<T>(
  query: string,
  variables?: Record<string, unknown>
): Promise<T> {
  const res = await fetch(`${BFF_URL}/graphql`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, variables }),
    cache: "no-store",
  });
  const json = await res.json();
  if (json.errors) throw new Error(json.errors[0].message);
  return json.data as T;
}

export const GET_AUDIT_EVENTS = `
  query GetAuditEvents($tenant_id: String!, $limit: Int) {
    auditEvents(tenant_id: $tenant_id, limit: $limit) {
      id event_type actor_id resource_id occurred_at
    }
  }
`;

export const GET_UBO_GRAPH = `
  query GetUBOGraph($tenant_id: String!, $app_id: String!) {
    uboGraph(tenant_id: $tenant_id, app_id: $app_id) {
      app_id
      nodes { id name node_type stake is_ubo }
      ubos  { id name node_type stake is_ubo }
    }
  }
`;

export const GET_TENANT = `
  query GetTenant($id: String!) {
    tenant(id: $id) { id name slug }
  }
`;
