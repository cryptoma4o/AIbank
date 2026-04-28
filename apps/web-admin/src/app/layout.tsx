import type { Metadata } from "next";

import "./globals.css";
import { ApolloProviderClient } from "@/components/ApolloProviderClient";
import { AppHeader } from "@/components/AppHeader";

export const metadata: Metadata = {
  title: "AIbank · Админ-панель банка",
  description:
    "Управление заявками, риск-оценкой, аудитом и конфигурацией тенанта",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="ru">
      <body className="min-h-screen bg-surface text-gray-900 antialiased">
        <ApolloProviderClient>
          <AppHeader />
          <main className="mx-auto max-w-7xl px-6 py-6">{children}</main>
        </ApolloProviderClient>
      </body>
    </html>
  );
}
