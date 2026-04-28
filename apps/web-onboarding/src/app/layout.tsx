import type { Metadata } from "next";

import "./globals.css";
import { ApolloProviderClient } from "@/components/ApolloProviderClient";
import { AppHeader } from "@/components/AppHeader";

export const metadata: Metadata = {
  title: "Открытие счёта — Банк",
  description: "Онлайн-открытие расчётного счёта для бизнеса",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ru">
      <body className="bg-surface min-h-screen antialiased text-gray-900">
        <ApolloProviderClient>
          <AppHeader />
          <main className="mx-auto max-w-5xl px-6 py-8">{children}</main>
          <footer className="mt-16 border-t border-gray-200 py-6 text-center text-sm text-gray-500">
            © 2024 Банк. Лицензия ЦБ РФ №0000. Все права защищены.
          </footer>
        </ApolloProviderClient>
      </body>
    </html>
  );
}
