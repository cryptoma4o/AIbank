"use client";

// AuthGuard — клиентский guard для защищённых роутов admin-панели.
//
// На монтировании синхронно проверяет наличие валидного access-токена и
// (опционально) принадлежность пользователя к одной из allowed-ролей.
// Если токена нет — редирект на /login; если роль не подходит — рисует
// ErrorBanner с сообщением, чтобы оператор увидел причину отказа.

import { useEffect, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";

import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { hasAnyRole, isAuthenticated } from "@/lib/auth";

interface Props {
  children: ReactNode;
  /** Если задан — внутрь пускаем только тех, чья роль входит в список. */
  allowRoles?: readonly string[];
}

export function AuthGuard({ children, allowRoles }: Props) {
  const router = useRouter();
  const [status, setStatus] = useState<"checking" | "authed" | "forbidden">(
    "checking"
  );

  useEffect(() => {
    if (!isAuthenticated()) {
      router.replace("/login");
      return;
    }
    if (allowRoles && allowRoles.length > 0) {
      if (!hasAnyRole(allowRoles)) {
        setStatus("forbidden");
        return;
      }
    }
    setStatus("authed");
  }, [router, allowRoles]);

  if (status === "checking") {
    return (
      <div className="flex justify-center py-16">
        <LoadingSpinner label="Проверка сессии…" />
      </div>
    );
  }
  if (status === "forbidden") {
    return (
      <div className="mx-auto max-w-2xl py-10">
        <ErrorBanner error="У вашей роли нет доступа к этому разделу. Обратитесь к администратору банка." />
      </div>
    );
  }
  return <>{children}</>;
}
