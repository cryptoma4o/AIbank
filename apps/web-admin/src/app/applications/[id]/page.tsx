import { gqlFetch, GET_UBO_GRAPH } from "@/lib/graphql";
import { UBOGraphView } from "@/components/ubo/UBOGraphView";
import type { UBOGraph } from "@/types";

export default async function ApplicationDetailPage({ params }: { params: { id: string } }) {
  let uboGraph: UBOGraph | null = null;
  const tenantId = "demo-bank";
  try {
    const data = await gqlFetch<{ uboGraph: UBOGraph }>(GET_UBO_GRAPH, {
      tenant_id: tenantId,
      app_id: params.id,
    });
    uboGraph = data.uboGraph;
  } catch { /* offline */ }

  return (
    <div>
      <h1 className="text-xl font-bold text-gray-900 mb-2">Заявка</h1>
      <p className="text-gray-400 font-mono text-sm mb-6">{params.id}</p>
      <div className="bg-white rounded-xl border border-gray-200 p-5">
        {uboGraph ? (
          <UBOGraphView graph={uboGraph} />
        ) : (
          <p className="text-gray-400 text-sm">Граф УБО недоступен (BFF offline).</p>
        )}
      </div>
    </div>
  );
}
