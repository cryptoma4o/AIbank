"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import type { CompanyType } from "@/types";

export function CompanyInfoForm() {
  const router = useRouter();
  const [form, setForm] = useState({ inn: "", ogrn: "", fullName: "", type: "ooo" as CompanyType });
  const [errors, setErrors] = useState<Partial<typeof form>>({});
  const [loading, setLoading] = useState(false);

  function validate() {
    const e: Partial<typeof form> = {};
    if (!/^\d{10}$|^\d{12}$/.test(form.inn)) e.inn = "ИНН должен содержать 10 или 12 цифр";
    if (!/^\d{13}$|^\d{15}$/.test(form.ogrn)) e.ogrn = "ОГРН должен содержать 13 или 15 цифр";
    if (!form.fullName.trim()) e.fullName = "Укажите наименование организации";
    return e;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const errs = validate();
    if (Object.keys(errs).length) { setErrors(errs); return; }
    setLoading(true);
    try {
      // POST to api-gateway → onboarding-orchestrator
      const res = await fetch("/api/applications", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(form),
      });
      const data = await res.json();
      router.push(`/status/${data.id}`);
    } catch {
      setErrors({ fullName: "Ошибка при создании заявки. Попробуйте ещё раз." });
    } finally {
      setLoading(false);
    }
  }

  return (
    <Card>
      <h2 className="text-xl font-semibold text-gray-900 mb-6">Сведения об организации</h2>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <div>
          <label className="text-sm font-medium text-gray-700 block mb-1.5">Форма собственности</label>
          <div className="flex gap-3">
            {(["ip", "ooo", "ao"] as CompanyType[]).map((t) => (
              <button key={t} type="button"
                onClick={() => setForm(f => ({ ...f, type: t }))}
                className={`px-4 py-2 rounded-lg border text-sm font-medium transition-colors
                  ${form.type === t ? "border-primary bg-primary/5 text-primary" : "border-gray-300 text-gray-600 hover:bg-gray-50"}`}>
                {t.toUpperCase()}
              </button>
            ))}
          </div>
        </div>
        <Input id="inn" label="ИНН" value={form.inn} onChange={e => setForm(f => ({ ...f, inn: e.target.value }))}
          error={errors.inn} placeholder="0000000000" maxLength={12} />
        <Input id="ogrn" label="ОГРН / ОГРНИП" value={form.ogrn} onChange={e => setForm(f => ({ ...f, ogrn: e.target.value }))}
          error={errors.ogrn} placeholder="0000000000000" maxLength={15} />
        <Input id="fullName" label="Полное наименование" value={form.fullName}
          onChange={e => setForm(f => ({ ...f, fullName: e.target.value }))} error={errors.fullName}
          placeholder='ООО "Название"' />
        <Button type="submit" disabled={loading} className="mt-2 self-start">
          {loading ? "Создание заявки…" : "Продолжить →"}
        </Button>
      </form>
    </Card>
  );
}
