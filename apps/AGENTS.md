<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# apps/

## Purpose

Фронтенд-приложения с пользовательским интерфейсом. Каждое — отдельный Next.js-проект (App Router), деплоится самостоятельно. Текущий статус: **директория пустая, приложения создаются в Фазе 1**.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Обзор слоя фронтенд-приложений: назначение, технологии, ссылки на документацию |

## Planned Subdirectories

| Директория | Назначение | Фаза |
|-----------|-----------|------|
| `web-onboarding/` | Next.js — клиентский кабинет (ИП/ООО заполняют заявку, загружают документы, общаются с чат-помощником) | Ф1 |
| `web-admin/` | Next.js — административная панель банка (комплаенс-офицер, оператор, администратор тенанта) | Ф1 |
| `courier-tablet/` | PWA для планшета курьера (выезд к клиенту, идентификация, сбор подписей) | Ф8 |
| `ops-dashboard/` | Внутренняя панель платформы (мониторинг тенантов, биллинг, admin SaaS) | Ф2+ |

## For AI Agents

### Working In This Directory

- Стек: **TypeScript 5+, Next.js 14 (App Router), Tailwind CSS, shadcn/ui, TanStack Query**.
- Нативные мобильные приложения — **не делаем в Год 1**. PWA достаточно (стратегическое решение, не пересматривать).
- Shared UI-компоненты — в `../packages/ui-kit/`, не дублировать.
- Типы для доменной модели берутся из `../packages/domain-model/` (TS-генерация из JSON Schema).
- Данные из BFF через GraphQL (`../services/bff-onboarding/`, `../services/bff-admin/`).
- Брендинг (цвета, шрифты, логотипы) — из конфигурации тенанта, не хардкодить.

### Testing Requirements

- Unit тесты компонентов: Vitest + React Testing Library
- E2e: Playwright (ночные на staging)
- Accessibility: проверка через axe-core

### Common Patterns

- Каждое приложение — отдельный `package.json` с зависимостями; shared через `packages/`
- Routing: App Router (Next.js 14), серверные компоненты где возможно
- State: TanStack Query для серверного состояния, Zustand для локального UI-состояния
- Локализация: только русский (ru-RU) в Год 1; i18n-архитектура закладывается для экспорта в ЕАЭС позже

## Dependencies

### Internal
- `../packages/ui-kit/` — shared React-компоненты
- `../packages/domain-model/` — типы доменной модели (TS)
- `../services/bff-onboarding/` — GraphQL BFF для `web-onboarding`
- `../services/bff-admin/` — GraphQL BFF для `web-admin`

### External
- Next.js 14, Tailwind CSS, shadcn/ui, TanStack Query, Zod (валидация форм)

<!-- MANUAL: -->
