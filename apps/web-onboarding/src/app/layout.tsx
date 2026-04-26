import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Открытие счёта — Банк",
  description: "Онлайн-открытие расчётного счёта для бизнеса",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ru">
      <body className="bg-surface min-h-screen font-sans antialiased">
        <header className="bg-white border-b border-gray-200 px-6 py-4">
          <div className="max-w-4xl mx-auto flex items-center gap-3">
            <div className="w-8 h-8 bg-primary rounded-lg" />
            <span className="font-semibold text-gray-900 text-lg">Банк · Бизнес</span>
          </div>
        </header>
        <main className="max-w-4xl mx-auto px-6 py-8">{children}</main>
        <footer className="border-t border-gray-200 mt-16 py-6 text-center text-sm text-gray-500">
          © 2024 Банк. Лицензия ЦБ РФ №0000. Все права защищены.
        </footer>
      </body>
    </html>
  );
}
