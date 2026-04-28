# Break-Glass Procedure

## Когда использовать

Только когда **все** условия выполнены:

1. У нашего инженера нет прямого доступа к prod-данным банка-клиента (это норма; см. `security-architecture.md` § 1 «Мы не доверяем сами себе»)
2. Идёт активный P0/P1 инцидент, разрешение которого требует прямого доступа
3. Стандартные контрмеры (через self-service банка, runbook'и) уже исчерпаны
4. Согласие банка получено или получится в течение 1 часа

Никогда не использовать для:
- «Я хочу проверить» / «удобнее посмотреть руками»
- Регулярных операций (для них есть `tenant-cli`, dashboards, audit API)
- Доступа к данным банка БЕЗ инцидента (это нарушение DPA)

Ссылки: [`security-architecture.md` § 3.3](../security-architecture.md), [ADR-0010 § «Audit log»](../adr/0010-billing-and-audit-log.md).

## Принципы

- **Time-limited.** Максимум 4 часа доступа за один break-glass-запрос
- **Approved.** Аппрув security-офицера + уведомление CISO банка-клиента (не «попросить разрешения», а «сообщить — у нас активный инцидент»)
- **Recorded.** Полная видеозапись сессии (через Zoom/Teams/банковский эквивалент); запись передаётся банку
- **Audited.** Отдельная audit-запись `platform.break_glass_*` с actor, scope, justification — неудаляемо (immutable per ADR-0010)
- **Post-mortem mandatory.** Без исключений; если break-glass был — пишется PM в течение 5 рабочих дней

## Процедура (детальная)

### Шаг 1. Запрос

Инициатор (на-call инженер) заполняет шаблон в `#sec-oncall` Mattermost-канале:

```
:alert: BREAK-GLASS REQUEST
- Incident: <ID>, link: <ссылка>
- Severity: P0 | P1
- Scope: <tenant=bank-alpha, resources=postgres+vault>
- Justification: <2-3 sentences почему стандартные пути не работают>
- Estimated duration: <≤ 4h>
- Engineer: <ваш ID>
- Backup engineer (наблюдатель): <второй инженер>
```

### Шаг 2. Аппрув security-офицера

Security-офицер на дежурстве (через ротацию) в течение **15 минут**:

- Подтверждает, что инцидент реален и стандартные пути исчерпаны
- Решает категорию scope (`read-only` / `read-write`)
- Запускает уведомление CISO банка
- Reply в треде `:white_check_mark: APPROVED, scope=read-only, expires=<UTC ts>`

При отказе — обоснование в треде; пытаемся другим путём.

### Шаг 3. Уведомление CISO банка

```
Subject: [URGENT] Break-glass access initiated for tenant <bank>
- Incident: <ID>
- Engineer: <FIO>
- Scope: <details>
- Duration: 4 hours starting <ts>
- Recording channel: <Zoom link>
- Post-mortem: will be delivered within 5 working days
```

Канал — согласованный per-tenant в `configs/tenants/<tenant>/contacts.yaml` (Telegram CISO + email DPO + дублирование в банковский SOC).

### Шаг 4. Активация доступа

`tenant-cli` создаёт временную privileged role с TTL.

```bash
# С нашей админ-машины
tenant-cli break-glass activate \
  --tenant=<bank-alpha> \
  --engineer=<engineer-id> \
  --scope=read-only \
  --duration=4h \
  --incident=<incident-id> \
  --justification-file=/tmp/justification-<id>.md \
  --approver=<security-officer-id> \
  --observer=<backup-engineer-id>

# Output:
#   audit_event_id: <uuid>
#   temp_credentials_path: vault://platform/break-glass/<uuid>
#   expires_at: 2026-04-26T05:30:00Z
#   recording_url: https://zoom.us/...
```

Под капотом:
- Создаётся short-lived Vault token с правами scope
- Audit-event `platform.break_glass_activated` записан с hash-chain (per ADR-0010)
- Таймер на auto-revocation через duration

### Шаг 5. Запись сессии

- Вся работа — через Zoom/Teams с активной записью
- В записи виден экран инженера и идёт verbal narration «сейчас читаю audit-events за период X», «проверяю миграции», и т.д.
- Запись сохраняется в защищённое хранилище платформы; передаётся банку через 24 часа

### Шаг 6. Работа

```bash
# Получить временные credentials
vault read platform/break-glass/<uuid>
export PG_BREAK_GLASS_PWD=$(...)

# Все операции через bastion-pod, не локальный psql
kubectl run break-glass-shell --rm -it --restart=Never \
  --image=bastion:latest \
  --env="PGPASSWORD=$PG_BREAK_GLASS_PWD" \
  -- bash

# В bastion-shell все команды логируются в audit
psql -h <pg-host> -U break_glass_<uuid> -d aibank -c \
  "SELECT * FROM tnt_<tenant>.applications WHERE id = '<application_id>'"
```

Соблюдается:
- Минимальная необходимая выборка (не `SELECT * FROM applications` без LIMIT)
- Никакого export в локальный файл инженера
- Никакого UPDATE/DELETE без явного approval второго инженера-наблюдателя

### Шаг 7. Завершение / автоматическое истечение

```bash
# Ручное завершение раньше срока (preferred)
tenant-cli break-glass deactivate --uuid=<uuid> --summary-file=/tmp/summary.md

# Автоматическое истечение через 4h — Vault revoke + audit-event
# platform.break_glass_expired
```

После завершения:
- Vault credentials revoked
- Network policy «break-glass» снимается
- Все запросы под `break_glass_<uuid>` остаются в `tnt_<tenant>.audit_events`
- Запись Zoom-сессии загружается в защищённое хранилище

### Шаг 8. Post-mortem (обязательно)

В течение 5 рабочих дней:

- Документ в `docs/postmortems/break-glass-<id>.md`
- Передан банку (CISO + DPO)
- Action items: что изменить, чтобы в следующий раз обойтись без break-glass
- Reviewed на еженедельном security-meeting

## Шаблон формы запроса

```markdown
# Break-Glass Request <id>

## Incident context
- Incident ID: <id>
- Severity: P0 | P1
- Started at: <ts>
- Symptom: <одна строка>
- Standard mitigations attempted: <список с результатом>

## Access requested
- Tenant: <bank-id>
- Scope: read-only | read-write
- Resources: postgres / vault / s3 / kafka / temporal
- Specific tables/buckets/topics: <если ограничено>
- Duration: <≤ 4h>

## Justification
<3-5 предложений: почему стандартные пути не работают>

## Engineer
- Primary: <FIO + Mattermost handle>
- Observer (mandatory): <FIO>
- Approver (security-officer): <FIO>

## Communication
- Recording channel: <Zoom link>
- Bank CISO notified: <ts + channel>
- DPO notified: <ts + channel>
- Mattermost thread: <ссылка>

## Expected outcome
<что ожидаем найти / сделать>

## Pre-PM checklist
- [ ] Audit-event `platform.break_glass_activated` записан
- [ ] Recording started
- [ ] Observer на связи
- [ ] Bank CISO acknowledge получен
```

## Шаблон Post-mortem

```markdown
# Break-Glass Post-Mortem <id>

- Date: YYYY-MM-DD
- Incident: <id>
- Engineer: <FIO>
- Observer: <FIO>
- Approver: <FIO>
- Bank: <name>
- Duration: HH:MM МСК — HH:MM МСК

## Что было сделано

<bullet list действий с timestamps>

## Что найдено

<root cause или текущие выводы>

## Использованные привилегии

| Команда / запрос | Время | Цель |
|---|---|---|
| ... | HH:MM | ... |

## Последствия

- Изменения данных: <yes/no, если yes — какие>
- Доступ к PII: <yes/no, если yes — какие записи и зачем>

## Action items (как избежать в следующий раз)

| # | Что | Owner | Due | Tracker |
|---|---|---|---|---|
| 1 | Добавить self-service kpi-dashboard для X | <name> | <date> | JIRA-XXX |

## Артефакты

- Recording: <link, доступ только security-officer + bank CISO>
- Audit events: `tnt_<tenant>.audit_events` WHERE `actor = 'break_glass_<uuid>'`
- Bastion logs: `<path>`

## Подписи

- Security-officer: <name>
- Engineer: <name>
- Reviewed by CISO bank: <name + date>
```

## Что НЕ break-glass

- **Поддержка self-service банка** — оператор банка делает сам через `web-admin`
- **Read-only access к не-чувствительным метаданным** — стандартный `support_engineer` role с MFA достаточен
- **Migration на failed tenant** — это `db-migration.md` runbook, нет нужды в break-glass

## Связанные документы

- [`security-architecture.md` § 3.3](../security-architecture.md) — формальная политика
- [`incident-response.md`](incident-response.md) — общий поток инцидентов; break-glass это специальная подпроцедура
- [ADR-0010](../adr/0010-billing-and-audit-log.md) — audit log immutability
- [`audit-log-integrity.md`](audit-log-integrity.md) — verification что audit не сломан
- DPA с банком (per-tenant в `configs/tenants/<tenant>/legal/dpa.md`) — юридическая база break-glass-доступа
