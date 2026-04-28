# web-onboarding

Next.js 14 (App Router) фронтенд для самообслуживания applicant'а на платформе
AIbank — авторизация, список заявок, создание новой заявки и просмотр деталей.

## Стек

- **Next.js 14 / React 18** — App Router, client components.
- **TypeScript strict**.
- **Apollo Client 3** — все доменные запросы идут в `bff-onboarding` через GraphQL.
- **react-hook-form + zod** — формы и client-side валидация.
- **Tailwind CSS** — стили, банковский light-theme (primary `#2563EB`).

## Страницы

| Путь                          | Назначение                                            |
| ----------------------------- | ----------------------------------------------------- |
| `/login`                      | Вход (REST на identity-service `POST /v1/auth/login`) |
| `/applications`               | Список заявок текущего пользователя (`myApplications`)|
| `/applications/new`           | Форма создания заявки (`submitApplication` mutation)  |
| `/applications/[id]`          | Детали заявки (state, documents, riskAssessment)      |

Корневой `/` редиректит на `/login`. Защищённые страницы обёрнуты `AuthGuard`,
который синхронно проверяет валидность JWT в `localStorage` и при отсутствии
сессии делает `router.replace("/login")`.

## Apollo-клиент

Singleton-клиент в `src/lib/apollo.ts` собирается из:

```text
[ authLink (setContext, Bearer JWT) ] → [ HttpLink (NEXT_PUBLIC_BFF_ONBOARDING_URL) ]
```

`authLink` синхронно читает access-токен из `localStorage` через
`getAccessToken()` и подмешивает его в заголовок `Authorization`. Резолверы
BFF берут JWT-claims через middleware и пробрасывают их в downstream-сервисы.

## Окружение

`.env.local.example` лежит в корне приложения и описывает доступные
переменные. Для локального запуска скопируйте его:

```bash
cp .env.local.example .env.local
```

| Переменная                          | Назначение                                  | Дефолт                              |
| ----------------------------------- | ------------------------------------------- | ----------------------------------- |
| `NEXT_PUBLIC_BFF_ONBOARDING_URL`    | GraphQL-эндпоинт `bff-onboarding`           | `http://localhost:8091/graphql`     |
| `NEXT_PUBLIC_IDENTITY_SERVICE_URL`  | REST-эндпоинт `identity-service` (login/me) | `http://localhost:8082`             |

## Запуск

```bash
cd apps/web-onboarding
npm install
npm run dev    # http://localhost:3000
npm run build  # production-сборка (standalone)
```

## Что не входит в скелет (TODO)

- Subdomain-based tenant detection — отложено до интеграции с `api-gateway`.
- OAuth/MFA — текущий слой ограничен JWT-логином identity-service.
- Загрузка документов — кнопка «Загрузить документ» на странице заявки
  показывает stub-уведомление; модальное окно с `uploadDocument` mutation
  планируется в следующей итерации.
- Чат с бизнес-клиентом (бывший `agent-conversational`) перенесён в
  отдельный экран и пока не интегрирован.
