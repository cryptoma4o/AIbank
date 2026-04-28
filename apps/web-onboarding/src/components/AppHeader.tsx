"use client";

// AppHeader — фиксированная шапка с лого, навигацией и кнопкой выхода.
// Кнопка выхода отображается только если есть JWT.

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { isAuthenticated, logout } from "@/lib/auth";

export function AppHeader() {
  const router = useRouter();
  const [authed, setAuthed] = useState(false);

  useEffect(() => {
    setAuthed(isAuthenticated());
  }, []);

  function handleLogout() {
    logout();
    setAuthed(false);
    router.replace("/login");
  }

  return (
    <header className="border-b border-gray-200 bg-white">
      <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
        <Link href={authed ? "/applications" : "/login"} className="flex items-center gap-3">
          <div className="h-8 w-8 rounded-lg bg-primary" aria-hidden="true" />
          <span className="text-lg font-semibold text-gray-900">Банк · Бизнес</span>
        </Link>
        {authed ? (
          <button
            type="button"
            onClick={handleLogout}
            className="text-sm font-medium text-gray-600 hover:text-primary"
          >
            Выйти
          </button>
        ) : null}
      </div>
    </header>
  );
}
