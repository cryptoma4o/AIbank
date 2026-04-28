"use client";

// ApolloProviderClient — обёртка ApolloProvider для App Router.
//
// Provider должен быть client-component, потому что singleton-клиент
// держит ссылку на window-зависимый authLink (см. lib/apollo.ts).

import { ApolloProvider } from "@apollo/client";
import type { ReactNode } from "react";

import { getApolloClient } from "@/lib/apollo";

export function ApolloProviderClient({ children }: { children: ReactNode }) {
  const client = getApolloClient();
  return <ApolloProvider client={client}>{children}</ApolloProvider>;
}
