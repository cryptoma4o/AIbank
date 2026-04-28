"use client";

// AuthGuard — клиентский guard для защищённых роутов.
//
// На монтировании синхронно проверяет наличие валидного access-токена в
// localStorage; если токена нет или он просрочен — редиректит на /login.
// Пока проверка не выполнена, рисует spinner, чтобы не мигать защищённым
// контентом.

import { useEffect, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";

import { isAuthenticated } from "@/lib/auth";
import { LoadingSpinner } from "@/components/LoadingSpinner";

export function AuthGuard({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [status, setStatus] = useState<"checking" | "authed">("checking");

  useEffect(() => {
    if (isAuthenticated()) {
      setStatus("authed");
      return;
    }
    router.replace("/login");
  }, [router]);

  if (status === "checking") {
    return (
      <div className="flex justify-center py-16">
        <LoadingSpinner label="Проверка сессии…" />
      </div>
    );
  }
  return <>{children}</>;
}
