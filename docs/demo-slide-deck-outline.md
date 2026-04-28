<!-- Parent: ./AGENTS.md -->

# Demo Slide-Deck Outline

Структура презентации для встречи с банком-партнёром (10-12 слайдов,
15-20 минут включая live demo). Привязано к [demo-script.md](./demo-script.md).

**Формат**: 16:9, no-frills, focus на content. Можно быстро собрать в
Google Slides / Keynote по этому outline'у.

---

## Slide 1: Title

```
AIbank
White-label онбординг-платформа для российских банков

[Логотип] | [Founder name] | [Дата встречи]
```

**Speaker note**: 30 сек. Рукопожатие, спасибо, кратко представить себя.

---

## Slide 2: Проблема

```
Онбординг юрлица в банке — 3-7 дней, 70% ручной работы.

  • KYC/KYB по 115-ФЗ: документы → проверки → решение
  • UBO graph (ФЗ-115 § 7.3): построение цепочек владения
  • Risk scoring: ручной для middle-bank
  • Open account в АБС: SOAP/MQ интеграции с легаси

Цена: ~₽5-15K на заявку. Доля automated decisions: 10-25%.
```

**Speaker note**: 1 мин. «Это где банки 30-200 места теряют деньги
и time-to-market против Сбера/Альфы».

---

## Slide 3: Решение

```
AIbank — automated KYC/KYB pipeline за 90 секунд

  ✓ Document Intake AI (vision-LLM): извлекает поля из PDF
  ✓ UBO Tracing AI: строит граф владения с обнаружением offshore
  ✓ Risk Scoring AI: explainable scoring (SHAP-style)
  ✓ Conversational AI: чат-бот для клиентских вопросов
  ✓ ABS-адаптеры (canonical → ЦФТ/Diasoft/RS-Bank)
  ✓ White-label: per-tenant брендинг, конфиги, риск-политики

Целевая доля automated decisions: 70-80%.
```

**Speaker note**: 1 мин. «Архитектура multi-tenant — один деплой
обслуживает несколько банков с полной изоляцией данных».

---

## Slide 4: Архитектура (high-level)

```
[Web-onboarding (Next.js)]    [Web-admin (Next.js)]
         │                            │
[BFF-onboarding (GraphQL)]   [BFF-admin (GraphQL)]
         │                            │
              [API Gateway]
                   │
   ┌───────┬───────┼───────┬───────────┐
[Identity] [Tenant] [Doc]   [Risk]   [Onboarding-orchestrator]
                     │       │              │ (Temporal)
              [AI-агенты × 6]                │
                     │       │              │
   ┌───────────────────────────────────────┐
   │ ext-egrul, ext-rosfinmon, ext-fssp, ext-spark │
   │ abs-adapter-cft / -diasoft / -rs-bank │
   └───────────────────────────────────────┘
            │              │
   [PostgreSQL]  [Kafka]  [Vault]  [Qdrant]  [MinIO]
```

**Speaker note**: 1 мин. «21 Go-микросервис + 6 Python AI-агентов +
2 Next.js фронтенда. Hybrid deployment — SaaS или on-prem».

---

## Slide 5: Live Demo

```
СЕЙЧАС покажем 3 сценария:

  1. ООО → low risk → auto-approve     (~90 сек)
  2. ИП → medium risk → manual review   (~90 сек)
  3. АО → 115-ФЗ match → declined        (~90 сек)

Запуск: ./demo/run-demo.sh
```

**Speaker note**: 30 сек. Переключение в терминал. Live demo занимает
~5 минут (slides 6-8 — это пометки на терминале).

---

## Slide 6: Demo Сценарий 1 — LLC happy path

(Slide остаётся фоном, зритель смотрит в терминал)

```
Чек-лист:
  ✓ AI извлёк 15 полей из паспорта + устава за 10 сек
  ✓ UBO построен (1 владелец 100%, residency=RU)
  ✓ Risk score 0.15 (low) — auto-approved
  ✓ ABS-CFT mock открыл счёт за 4 сек
  ✓ Audit log 7 событий, hash chain валиден
```

---

## Slide 7: Demo Сценарий 2 — IP manual review

```
Чек-лист:
  ✓ AI обнаружил отсутствие ОГРНИП-выписки
  ✓ Risk score 0.55 (medium) — escalation
  ✓ Operator UI: web-admin, decision modal
  ✓ Operator комментарий 20+ символов в audit
  ✓ Account opened после approve
```

---

## Slide 8: Demo Сценарий 3 — AO rejected

```
Чек-лист:
  ✓ Foreign offshore без disclosure → risk +0.4
  ✓ ext-rosfinmon fuzzy match (синт. 115-ФЗ список)
  ✓ Risk score 0.95 (high) → auto-decline
  ✓ Клиенту: generic message (115-ФЗ § 7 compliance)
  ✓ Compliance-officer: full reason в audit + UI
```

---

## Slide 9: Security & Compliance

```
                Compliance:
  ✓ Hash chain (SHA-256) + Ed25519 signature
  ✓ Append-only triggers в PostgreSQL
  ✓ audit-verifier CLI для tamper detection
  ✓ Vault для секретов (HA + AppRole)
  ✓ mTLS Istio между сервисами (zero-trust)
  ✓ Schema-per-tenant изоляция

Готовы к 115-ФЗ, 375-П, 499-П, 152-ФЗ, 187-ФЗ, 63-ФЗ
(см. docs/compliance-map.md).
```

**Speaker note**: 1.5 мин. **Live**: запустить `audit-verifier` чтобы
показать "Цепочка ЦЕЛАЯ" → симуляция tampering → "TAMPERING ALERT P0".

---

## Slide 10: Pilot-readiness Gap (открыто)

```
Что готово сейчас (Pre-MVP):
  ✅ 21 Go-микросервис + 6 AI-агентов (тесты зелёные)
  ✅ 14 ADR, 12 OpenAPI specs, 2 GraphQL BFF
  ✅ Audit-bundle для ИБ-аудита банка

Что ждёт внешних ресурсов (4-8 недель lead-time):
  🟡 ФНС/ЕГРЮЛ договор через Минцифры
  🟡 КриптоПро/VipNet лицензии (УКЭП)
  🟡 GPU для vLLM (Gemma 4 / Qwen 3.5 / T-pro)
  🟡 Pen-test внешним подрядчиком

Code → production: flip *_LIVE=true ENV.
```

**Speaker note**: 1.5 мин. «Прозрачность — наш принцип. Полный gap-list
в pilot-readiness.md».

---

## Slide 11: Pricing & Pilot

```
Pricing model (предлагаемая):

  • Setup fee:        ₽X (one-time, on-prem deployment)
  • Per-event:        ₽Y per onboarding application
  • Maintenance:      ₽Z/мес (24×7 support, security updates)

Пилотные условия:
  • 3 месяца, до N заявок, скидка 50% на per-event
  • Безлимитный feedback channel (Slack/Telegram с командой)
  • Quarterly business review

Готовы:
  • LOI шаблон
  • DPA template (152-ФЗ)
  • SLA template (99.5% uptime для пилота)
```

**Speaker note**: 1.5 мин. Конкретные числа подставить под
банк-партнёра.

---

## Slide 12: Следующие шаги

```
1. ИБ-аудит: отправляем audit-bundle.md (1 неделя на review)
2. DevOps deep-dive: ADR-0013/0014 → встреча с DevOps командой
3. Sandbox staging: 1-2 недели на self-validation
4. LOI signing: 1-2 недели после positive feedback
5. Pilot kickoff: T+0 (после LOI)

Контакты:
  Founder: [name] [email]
  Tech-lead: [name] [email]
  Repository (приватный): [github URL после NDA]
```

**Speaker note**: 1 мин. Конкретные имена + dates.

---

## Slide 13 (опциональный): Q&A

```
Спасибо! Вопросы?

Frequent: цена / on-prem vs SaaS / data residency / SLA / pen-test history
```

---

## Tips для производительности

- **Не показывать код** — банкам это неинтересно. Показывайте flow,
  audit-log, comply checks.
- **Запустить терминал в FullScreen** до встречи. Заранее настроить
  font size 18+ для projector.
- **2 окна**: одно с `./demo/run-demo.sh`, второе с web-admin
  (`http://localhost:3001`).
- **Заранее открыть** `docs/audit-bundle.md`, `docs/pilot-readiness.md`
  в редакторе для быстрого reference на вопросы.
- **Time check** на slide 5 (15-минутная отметка) — если запаздываем,
  пропустить slide 9 walkthrough audit-verifier и сослаться на
  audit-bundle.

---

## Что взять с собой на встречу

- Ноутбук с запущенным `make up` за 30 минут до встречи
- HDMI/USB-C переходники
- Распечатанный slide-deck (опционально, для compliance-officer
  старшего возраста)
- USB stick с `audit-bundle.md` + ADR-0013/0014 + pilot-readiness.md
  (если банк хочет оставить себе)
- LOI / DPA / SLA template'ы в PDF
