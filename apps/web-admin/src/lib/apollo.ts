"use client";

// apollo.ts — фабрика ApolloClient для web-admin.
//
// HttpLink идёт в bff-admin (NEXT_PUBLIC_BFF_ADMIN_URL).  authLink через
// `setContext` подмешивает Bearer-JWT из localStorage в каждый запрос.
// Resolvers BFF читают этот заголовок в JWT-middleware
// (см. services/bff-admin/internal/auth) и прикладывают AuthContext к
// graphql.ResolveParams.Context.

import {
  ApolloClient,
  ApolloLink,
  HttpLink,
  InMemoryCache,
  from,
} from "@apollo/client";
import { setContext } from "@apollo/client/link/context";

import { getAccessToken } from "./auth";

const DEFAULT_BFF_URL = "http://localhost:8092/graphql";

function bffUrl(): string {
  const fromEnv = process.env.NEXT_PUBLIC_BFF_ADMIN_URL;
  return fromEnv && fromEnv.length > 0 ? fromEnv : DEFAULT_BFF_URL;
}

/** authLink: setContext добавляет Authorization, если токен есть. */
const authLink: ApolloLink = setContext((_operation, prevContext) => {
  const token = getAccessToken();
  const headers = (prevContext.headers as Record<string, string>) ?? {};
  if (token) {
    return {
      headers: {
        ...headers,
        authorization: `Bearer ${token}`,
      },
    };
  }
  return { headers };
});

let clientSingleton: ApolloClient<unknown> | null = null;

/**
 * Возвращает singleton ApolloClient.  Singleton нужен, чтобы Apollo cache
 * переживал переходы между страницами и React Strict Mode double-mount.
 */
export function getApolloClient(): ApolloClient<unknown> {
  if (clientSingleton) return clientSingleton;
  const httpLink = new HttpLink({ uri: bffUrl(), credentials: "omit" });
  clientSingleton = new ApolloClient({
    link: from([authLink, httpLink]),
    cache: new InMemoryCache({
      typePolicies: {
        // Apollo нужно знать, что Application и AuditEvent имеют id —
        // иначе он по дефолту использует `__typename:id`, что нам как раз
        // и нужно, но запас на будущее (cursor-pagination в audit).
        Query: {
          fields: {
            applications: { merge: false },
            auditEvents: { merge: false },
          },
        },
      },
    }),
    defaultOptions: {
      watchQuery: { fetchPolicy: "cache-and-network", errorPolicy: "all" },
      query: { fetchPolicy: "network-only", errorPolicy: "all" },
      mutate: { errorPolicy: "all" },
    },
  });
  return clientSingleton;
}
