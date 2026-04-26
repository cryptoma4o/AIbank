import { Badge } from "@/components/ui/Badge";

export default function ApplicationsPage() {
  return (
    <div>
      <h1 className="text-xl font-bold text-gray-900 mb-6">Заявки</h1>
      <div className="bg-white rounded-xl border border-gray-200 p-8 text-center">
        <p className="text-gray-400 text-sm">Список заявок доступен после подключения к onboarding-orchestrator.</p>
        <div className="flex gap-2 justify-center mt-4 flex-wrap">
          {["draft", "validating", "auto_approved", "manual_review", "completed", "rejected"].map(s => (
            <Badge key={s} status={s} />
          ))}
        </div>
      </div>
    </div>
  );
}
