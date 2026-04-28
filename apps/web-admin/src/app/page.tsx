import { redirect } from "next/navigation";

// «Корневой» путь admin-панели всегда уводит в раздел заявок: это первое,
// что ожидает увидеть оператор после логина.  Если токена нет — AuthGuard
// в /applications перенаправит на /login.
export default function HomePage() {
  redirect("/applications");
}
