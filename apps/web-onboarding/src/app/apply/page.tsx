import { StepIndicator } from "@/components/ui/StepIndicator";
import { CompanyInfoForm } from "@/components/forms/CompanyInfoForm";

export default function ApplyPage() {
  return (
    <div>
      <StepIndicator current={0} />
      <h1 className="text-2xl font-bold text-gray-900 mb-2">Открытие расчётного счёта</h1>
      <p className="text-gray-500 mb-8">Шаг 1 из 5 — Сведения об организации</p>
      <CompanyInfoForm />
    </div>
  );
}
