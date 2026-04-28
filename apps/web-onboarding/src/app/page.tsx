import { redirect } from "next/navigation";

// Корневой роут: пока нет server-side checking JWT (skeleton), отправляем
// в /login.  AuthGuard на /applications сам разберётся с валидной сессией.
export default function HomePage() {
  redirect("/login");
}
