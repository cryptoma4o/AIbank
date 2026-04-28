// ErrorBanner — единый компонент для отображения ошибок (GraphQL/REST).
//
// Принимает либо строку, либо Apollo ApolloError-подобный объект (нам
// достаточно полей `message` и `graphQLErrors`).

import type { ApolloError } from "@apollo/client";

type ErrorLike = string | Error | ApolloError | null | undefined;

function extractMessage(err: ErrorLike): string | null {
  if (!err) return null;
  if (typeof err === "string") return err;
  // ApolloError помечает себя через graphQLErrors/networkError.
  const apollo = err as ApolloError;
  if (apollo.graphQLErrors?.length) {
    return apollo.graphQLErrors.map((e) => e.message).join("; ");
  }
  if (apollo.networkError) {
    return `Ошибка сети: ${apollo.networkError.message}`;
  }
  return (err as Error).message ?? "Неизвестная ошибка";
}

export function ErrorBanner({ error }: { error: ErrorLike }) {
  const message = extractMessage(error);
  if (!message) return null;
  return (
    <div
      role="alert"
      className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-danger"
    >
      <strong className="font-medium">Ошибка. </strong>
      <span>{message}</span>
    </div>
  );
}
