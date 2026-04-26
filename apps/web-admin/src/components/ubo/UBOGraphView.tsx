import type { UBOGraph } from "@/types";

export function UBOGraphView({ graph }: { graph: UBOGraph }) {
  return (
    <div>
      <h3 className="text-sm font-semibold text-gray-700 mb-3">Структура владения</h3>
      {graph.ubos.length > 0 && (
        <div className="mb-4 p-3 bg-orange-50 border border-orange-200 rounded-lg">
          <p className="text-xs font-semibold text-orange-700 mb-2">УБО (доля ≥ 25%)</p>
          <ul className="flex flex-col gap-1">
            {graph.ubos.map((n) => (
              <li key={n.id} className="text-sm text-orange-800 flex items-center gap-2">
                <span className="w-2 h-2 bg-orange-500 rounded-full" />
                {n.name} — {n.stake.toFixed(1)}%
              </li>
            ))}
          </ul>
        </div>
      )}
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-gray-500 border-b border-gray-200">
              <th className="pb-2 font-medium">Участник</th>
              <th className="pb-2 font-medium">Тип</th>
              <th className="pb-2 font-medium">Доля %</th>
              <th className="pb-2 font-medium">УБО</th>
            </tr>
          </thead>
          <tbody>
            {graph.nodes.map((n) => (
              <tr key={n.id} className="border-b border-gray-100 hover:bg-gray-50">
                <td className="py-2.5 font-medium text-gray-900">{n.name}</td>
                <td className="py-2.5 text-gray-500">{n.node_type === "person" ? "Физлицо" : "Юрлицо"}</td>
                <td className="py-2.5">{n.stake.toFixed(1)}%</td>
                <td className="py-2.5">{n.is_ubo ? <span className="text-orange-600 font-medium">Да</span> : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
