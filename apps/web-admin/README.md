# web-admin

Внутренняя админ-панель для сотрудников банка-клиента: управление
заявками, риск-оценкой, аудит-журналом и просмотр конфигурации тенанта.

## Архитектура

- **Next.js 14 App Router**, страницы — client-components с Apollo Client.
- Поверх **bff-admin** (GraphQL, `:8092`) — единственного даунстрима.
- Авторизация — JWT от **identity-service** (`:8082`, REST `/v1/auth/login`).
- Доступ только для ролей `platform.admin`, `bank.admin`,
  `bank.compliance_officer`, `bank.operator`. Остальные (например,
  `applicant`) получают сообщение об отказе.

## Страницы

| Путь | Кто видит | Что делает |
|---|---|---|
| `/login` | публично | Логин (email + пароль + tenant_id), проверка ролей |
| `/applications` | все admin-роли | Список заявок тенанта с фильтрами (состояние, ИНН, даты) |
| `/applications/[id]` | все admin-роли | Карточка заявки: атрибуты, риск-оценка, факторы, документы, audit timeline; принятие решений (approve/EDD/decline/escalate) для compliance_officer + admin |
| `/audit` | `bank.admin`, `platform.admin` | Журнал аудит-событий тенанта с фильтрами и expandable payload |
| `/tenant` | `bank.admin`, `platform.admin` | Read-only вкладочный просмотр `tenant.yaml` |

Запись `tenant.yaml` (бренд, риск-политика и т. п.) идёт через GitOps,
не через GraphQL — см. ADR-0003.

## Окружение

Скопируйте `.env.local.example` в `.env.local` и при необходимости
переопределите URL-ы:

```bash
cp .env.local.example .env.local
```

| Переменная | Значение по умолчанию |
|---|---|
| `NEXT_PUBLIC_BFF_ADMIN_URL` | `http://localhost:8092/graphql` |
| `NEXT_PUBLIC_IDENTITY_SERVICE_URL` | `http://localhost:8082` |

## Запуск

```bash
npm install
npm run dev          # http://localhost:3001
npm run build        # production-сборка (output: standalone)
npm run type-check   # tsc --noEmit
```

## Зависимости

- `@apollo/client` 3.11+, `graphql` 16
- `react-hook-form` + `zod` для форм и валидации
- `date-fns` (форматирование таймстемпов в audit log)
- Tailwind CSS (банковская монохромная палитра, акцент `#1E40AF`)

## Известные ограничения skeleton-этапа

- Документы и подробные данные заявителя пока не отдаются `bff-admin`
  (см. ADR-0003) — соответствующие блоки рисуют заглушку.
- Мутация `updateApplicationDecision` ожидает, что эмиссию audit-события
  делает backend; UI её не дублирует.
- Tenant config доступен только для чтения; запись — через PR в
  `configs/tenants/<tenant_id>/tenant.yaml`.
- Поиск по INN на странице заявок применяется клиентски, потому что схема
  `bff-admin` v0.1 не имеет поля `inn` в `ApplicationsFilter`.
