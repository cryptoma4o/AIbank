import Link from "next/link";

const NAV = [
  { href: "/dashboard",     label: "Дашборд",     icon: "⬛" },
  { href: "/applications",  label: "Заявки",       icon: "📋" },
  { href: "/audit",         label: "Аудит",        icon: "🔍" },
  { href: "/tenants",       label: "Тенанты",      icon: "🏦" },
];

export function Sidebar() {
  return (
    <aside className="w-56 bg-sidebar text-white flex flex-col shrink-0">
      <div className="px-4 py-5 border-b border-white/10">
        <div className="flex items-center gap-2">
          <div className="w-7 h-7 bg-primary rounded-md" />
          <span className="font-semibold text-sm">AIbank Admin</span>
        </div>
      </div>
      <nav className="flex-1 px-2 py-4 flex flex-col gap-0.5">
        {NAV.map(({ href, label, icon }) => (
          <Link key={href} href={href}
            className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-white/70 hover:text-white hover:bg-white/10 transition-colors">
            <span className="text-base">{icon}</span>
            {label}
          </Link>
        ))}
      </nav>
      <div className="px-4 py-3 border-t border-white/10 text-xs text-white/40">
        v0.1.0 · Pre-MVP
      </div>
    </aside>
  );
}
