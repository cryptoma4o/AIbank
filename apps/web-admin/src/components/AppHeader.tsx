"use client";

// AppHeader — фиксированная шапка с навигацией и кнопкой выхода.
//
// В отличие от web-onboarding, у оператора несколько разделов: Заявки,
// Аудит, Тенант.  Подсветка активного раздела через usePathname.

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { getAccessToken, getRoles, isAuthenticated, logout } from "@/lib/auth";
import { roleLabel } from "@/lib/roles";
import { decodeJwtPayload } from "@/lib/auth";

interface NavItem {
  href: string;
  label: string;
}

const NAV: NavItem[] = [
  { href: "/applications", label: "Заявки" },
  { href: "/audit", label: "Аудит" },
  { href: "/tenant", label: "Тенант" },
];

export function AppHeader() {
  const router = useRouter();
  const pathname = usePathname();
  const [authed, setAuthed] = useState(false);
  const [roleText, setRoleText] = useState<string>("");

  useEffect(() => {
    const ok = isAuthenticated();
    setAuthed(ok);
    if (ok) {
      const roles = getRoles();
      const primary = roles[0];
      const tok = getAccessToken();
      const payload = tok ? decodeJwtPayload(tok) : null;
      const sub = payload?.sub ?? "";
      setRoleText(
        primary ? `${roleLabel(primary)}${sub ? ` · ${sub.slice(0, 8)}…` : ""}` : ""
      );
    }
  }, [pathname]);

  function handleLogout() {
    logout();
    setAuthed(false);
    router.replace("/login");
  }

  // На /login шапку не показываем — там пустой layout.
  if (pathname === "/login") return null;

  return (
    <header className="border-b border-gray-200 bg-white">
      <div className="mx-auto flex max-w-7xl items-center justify-between gap-6 px-6 py-3">
        <Link
          href={authed ? "/applications" : "/login"}
          className="flex items-center gap-3"
        >
          <div className="h-7 w-7 rounded-md bg-primary" aria-hidden="true" />
          <span className="text-base font-semibold text-gray-900">
            AIbank · Админ
          </span>
        </Link>
        {authed ? (
          <nav className="flex flex-1 items-center gap-1 text-sm">
            {NAV.map((item) => {
              const active =
                pathname === item.href || pathname.startsWith(`${item.href}/`);
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={`rounded-md px-3 py-1.5 font-medium transition-colors ${
                    active
                      ? "bg-primary-50 text-primary"
                      : "text-gray-600 hover:bg-gray-50 hover:text-gray-900"
                  }`}
                >
                  {item.label}
                </Link>
              );
            })}
          </nav>
        ) : (
          <span aria-hidden="true" />
        )}
        {authed ? (
          <div className="flex items-center gap-4">
            {roleText ? (
              <span className="text-xs text-gray-500">{roleText}</span>
            ) : null}
            <button
              type="button"
              onClick={handleLogout}
              className="text-sm font-medium text-gray-600 hover:text-primary"
            >
              Выйти
            </button>
          </div>
        ) : null}
      </div>
    </header>
  );
}
