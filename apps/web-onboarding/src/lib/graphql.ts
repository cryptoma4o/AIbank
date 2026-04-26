const BFF_URL = process.env.BFF_URL ?? "http://localhost:8094";

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

export const GET_APPLICATION = `
  query GetApplication($id: String!) {
    application(id: $id) {
      id
      tenant_id
      status
      inn
      ogrn
    }
  }
`;
