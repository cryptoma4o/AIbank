# Tenant Configuration

Документ описывает, **что и как настраивает банк-клиент** в платформе. Конфигурация — главный архитектурный артефакт, отделяющий код от бизнес-правил банка.

---

## 1. Принципы конфигурации

- **Configuration over code.** Любое поведение, которое потенциально может отличаться у разных банков, выносится в конфигурацию. Не «у нас в коде есть переключатель», а «банк описывает в YAML, что делать»
- **Декларативно, не императивно.** Банк говорит «что должно быть», не «как сделать»
- **Версионируется как код.** Конфигурация в Git, изменения через PR, история сохраняется
- **Валидируется до применения.** JSON Schema + бизнес-валидация + dry-run
- **GitOps-доставка.** Изменения применяются через ArgoCD, не через UI и не через прямой `kubectl`
- **Секреты отдельно.** В конфигурации — ссылки на Vault, никогда не сами секреты
- **Default-конфигурация — самая строгая.** Расслабления — явные

---

## 2. Структура конфигурации тенанта

```
configs/tenants/{tenant-id}/
├── tenant.yaml                  # Основные параметры
├── branding/
│   ├── theme.json               # Цвета, шрифты, логотипы
│   └── content.yaml             # Тексты писем, уведомлений, ошибок
├── workflows/
│   ├── ip.yaml                  # Воркфлоу для ИП
│   ├── llc.yaml                 # Воркфлоу для ООО
│   └── jsc.yaml                 # Воркфлоу для АО
├── risk-policy/
│   ├── rules.yaml               # Декларативные правила
│   ├── thresholds.yaml          # Пороги скоринга
│   └── blocked-okveds.yaml      # Заблокированные ОКВЭДы
├── integrations/
│   ├── abs.yaml                 # АБС: тип, endpoint, маппинг
│   ├── external.yaml            # ЕГРЮЛ, СПАРК, БКИ
│   └── notifications.yaml       # Email, SMS, Push провайдеры
├── ai/
│   ├── models.yaml              # Какие модели использовать
│   └── prompts.yaml             # Кастомные промпты (поверх дефолтных)
├── sla.yaml                     # SLA-цели
├── compliance/
│   ├── consents.yaml            # Тексты согласий
│   ├── document-templates/      # Шаблоны договоров банка
│   └── retention.yaml           # Сроки хранения (если отличаются от дефолта)
└── security/
    └── access-policies.yaml     # Кто к чему имеет доступ
```

Каждый файл — самостоятельная единица с отдельной schema. Изменения проходят валидацию по своей schema перед применением.

---

## 3. tenant.yaml — основные параметры

```yaml
# Основной файл конфигурации тенанта
schema_version: "1.0"

tenant:
  id: "tnt_alfa_bank"
  name: "Альфа-Банк"
  legal_name: "АО Альфа-Банк"
  bik: "044525593"
  inn: "7728168971"
  
  status: "active"  # active | trial | suspended | archived
  
  contract:
    contract_id: "AB-2026-001"
    start_date: "2026-04-01"
    end_date: "2029-04-01"
    pricing_tier: "standard"
  
  deployment:
    mode: "on_prem"  # saas | on_prem | hybrid
    region: "ru-central"
    data_residency: ["RU"]
    
  contacts:
    technical_lead:
      name: "Иванов И.И."
      email: "ivanov@alfa-bank.ru"
      phone: "+7 495 123-45-67"
    compliance_officer:
      name: "Петров П.П."
      email: "compliance@alfa-bank.ru"
    business_owner:
      name: "Сидоров С.С."
      email: "sidorov@alfa-bank.ru"
      
  features:
    enabled:
      - "ip_onboarding"
      - "llc_onboarding"
      - "courier_channel"
      - "biometry_esia"
    disabled:
      - "jsc_onboarding"  # пока не запускаем
      - "mobile_app"
      
  limits:
    max_concurrent_applications: 10000
    max_documents_per_application: 50
    max_application_age_days: 90  # после 90 дней без активности → archived
```

---

## 4. workflows/llc.yaml — воркфлоу для ООО

```yaml
schema_version: "1.0"
workflow_name: "llc_onboarding"

# Какие шаги входят в воркфлоу и в каком порядке
steps:
  - id: "identify_applicant"
    type: "identity_verification"
    required: true
    methods:
      - "esia"      # приоритет 1
      - "ukep"      # приоритет 2
      - "courier"   # fallback
    timeout_minutes: 60
    
  - id: "collect_documents"
    type: "document_collection"
    required_documents:
      - type: "charter"
        description: "Устав ООО (последняя редакция)"
        required: true
        formats: ["pdf", "image/jpeg", "image/png"]
        max_size_mb: 50
      - type: "protocol_director_appointment"
        description: "Решение/протокол о назначении ЕИО"
        required: true
      - type: "passport"
        description: "Паспорт директора"
        required: true
        skip_if: "applicant_passport_verified_via_esia"
    optional_documents:
      - type: "accountant_appointment"
        description: "Приказ о назначении главбуха"
        
  - id: "extract_data"
    type: "ai_document_intake"
    parallel: true  # выполняется параллельно с egrul_lookup
    
  - id: "egrul_lookup"
    type: "external_data_fetch"
    source: "egrul"
    cache_ttl_hours: 24
    parallel: true
    
  - id: "reconcile"
    type: "ai_reconciliation"
    requires: ["extract_data", "egrul_lookup"]
    on_critical_discrepancy: "request_clarification"  # alternatives: block, escalate
    
  - id: "trace_ubo"
    type: "ai_ubo_tracing"
    max_recursion_depth: 5
    on_foreign_entity: "request_documents"
    
  - id: "screen_rosfinmon"
    type: "external_screening"
    source: "rosfinmon"
    parallel: true
    on_match: "escalate_high_risk"
    
  - id: "score_risk"
    type: "risk_scoring"
    requires: ["reconcile", "trace_ubo", "screen_rosfinmon"]
    
  - id: "decision"
    type: "decision_gate"
    auto_approve_if:
      - "risk.category == 'low'"
      - "screening.has_matches == false"
      - "reconciliation.status in ['match', 'minor_discrepancies']"
      - "ubo.unresolved_branches.length == 0"
      - "legal_entity.has_foreign_owners == false"
    otherwise: "manual_review"
    
  - id: "open_account"
    type: "abs_account_opening"
    requires: ["decision.approved"]
    products:
      - "current_rub"
      - "online_banking"
    
  - id: "notify_client"
    type: "client_notification"
    channels: ["email", "sms"]

# Обработка ошибок на уровне воркфлоу
error_handling:
  default_retry: 3
  default_timeout_minutes: 10
  on_step_failure:
    "extract_data": "retry_with_human_fallback"
    "egrul_lookup": "retry_with_backoff"
    "open_account": "saga_compensate"
    
# SLA для воркфлоу в целом
sla:
  reserved_account_max_minutes: 5
  decision_max_hours: 24
  full_completion_max_hours: 72
```

---

## 5. risk-policy/rules.yaml — декларативные правила

```yaml
schema_version: "1.0"

# Жёсткие правила, которые срабатывают независимо от ML-скоринга
hard_rules:
  - id: "ROSFINMON_MATCH"
    description: "Совпадение с перечнем Росфинмониторинга"
    condition: "screening.rosfinmon.has_matches == true"
    action: "set_high_risk"
    block_auto_approval: true
    
  - id: "BANKRUPTCY"
    description: "В процедуре банкротства"
    condition: "legal_entity.is_in_bankruptcy == true"
    action: "decline_recommended"
    
  - id: "LIQUIDATION"
    description: "В процессе ликвидации"
    condition: "legal_entity.is_in_liquidation == true"
    action: "decline_recommended"
    
  - id: "MASS_DIRECTOR"
    description: "Массовый руководитель (>5 действующих юрлиц)"
    condition: "director.active_legal_entities_count > 5"
    action: "set_medium_risk"
    
  - id: "MASS_REGISTRATION_ADDRESS"
    description: "Адрес массовой регистрации"
    condition: "legal_entity.legal_address.is_mass_registration == true"
    action: "set_medium_risk"
    
  - id: "FRESH_COMPANY"
    description: "Компания младше 6 месяцев"
    condition: "legal_entity.age_months < 6"
    action: "set_medium_risk"
    
  - id: "FOREIGN_UBO"
    description: "Иностранный конечный бенефициар"
    condition: "ubo.has_foreign_persons == true"
    action: "set_medium_risk"
    require_edd: true  # enhanced due diligence
    
  - id: "BLOCKED_OKVED"
    description: "Заблокированный ОКВЭД"
    condition: "legal_entity.okveds.main.code in tenant.blocked_okveds"
    action: "decline_recommended"
    
  - id: "FSSP_ACTIVE_PROCEEDINGS"
    description: "Активные исполнительные производства на крупную сумму"
    condition: "screening.fssp.active_amount_total > 5000000"  # 5 млн руб.
    action: "set_medium_risk"

# Действия после срабатывания правила
actions:
  set_high_risk:
    risk_category: "high"
    block_auto_approval: true
    require_human_review: true
    
  set_medium_risk:
    risk_category: "medium"
    block_auto_approval: true
    require_human_review: true
    
  decline_recommended:
    risk_category: "high"
    recommendation: "decline"
    require_human_review: true
    
# Приоритет правил при множественных срабатываниях
# (более строгое выигрывает)
priority_order:
  - "decline_recommended"
  - "set_high_risk"
  - "set_medium_risk"
```

---

## 6. risk-policy/thresholds.yaml — пороги скоринга

```yaml
schema_version: "1.0"

# Пороги категорий по результатам ML-скоринга (CatBoost)
score_thresholds:
  low_max: 0.35      # score <= 0.35 → low
  medium_max: 0.65   # 0.35 < score <= 0.65 → medium
                     # score > 0.65 → high

# Что нужно для попадания в "зелёную зону" (auto-approve)
auto_approve_criteria:
  required_all:
    - "risk.category == 'low'"
    - "screening.has_any_match == false"
    - "reconciliation.has_critical_discrepancies == false"
    - "ubo.has_unresolved_branches == false"
    - "ubo.has_foreign_persons == false"
    - "document.all_extractions_high_confidence == true"
    - "legal_entity.is_resident == true"
  
# Override-механика: банк может для определённых сегментов
# ослабить требования (например, для ИП)
overrides:
  for_legal_entity_type:
    "IP":
      auto_approve_criteria:
        required_all:
          - "risk.category in ['low', 'medium']"
          - "screening.has_any_match == false"
```

---

## 7. risk-policy/blocked-okveds.yaml

```yaml
schema_version: "1.0"

# ОКВЭД, по которым банк не открывает счета (или принимает с особым вниманием)
blocked:
  - code: "92"
    name: "Деятельность по организации и проведению азартных игр"
    reason: "internal_policy"
    
  - code: "20.51"
    name: "Производство взрывчатых веществ"
    reason: "license_required"
    require_documents: ["activity_license"]
    
medium_risk_okveds:
  - code: "46.7"
    name: "Торговля оптовая специализированная прочая"
    note: "Часто используется как прикрытие"
    
  - code: "70.22"
    name: "Консультирование по вопросам управления"
    note: "Потенциал для серых схем"
    
# ОКВЭДы, требующие дополнительных документов
require_additional_docs:
  - prefix: "84"  # государственное управление
    docs: ["state_registration_certificate"]
  - prefix: "65"  # страхование
    docs: ["insurance_license"]
```

---

## 8. integrations/abs.yaml — настройки АБС

```yaml
schema_version: "1.0"

primary_abs:
  type: "cft"          # cft | diasoft | rs_bank | csabs
  version: "10.x"
  
  connection:
    endpoint: "https://abs-prod.alfa-bank.internal/api"
    auth_method: "mtls"
    cert_ref: "vault://kv/tenants/alfa-bank/abs-cert"
    timeout_seconds: 30
    
  account_types:
    current_rub:
      abs_code: "40702810"  # балансовый счёт ООО в рублях
      activation_required: true
    current_usd:
      abs_code: "40702840"
      
  client_creation:
    requires_kpp: true
    requires_okveds: true
    auto_assign_branch: true
    
  signature_card:
    enabled: true
    digital_signing_supported: true
    
  reservation:
    enabled: true
    duration_days: 30
    
# Маппинг наших статусов на статусы АБС
status_mapping:
  "active": "АКТИВНЫЙ"
  "blocked": "ЗАБЛОКИРОВАН"
  "closed": "ЗАКРЫТ"
  
# Поведение при ошибках адаптера
error_handling:
  on_timeout: "retry_3_times"
  on_5xx: "escalate_to_devops"
  on_validation_error: "block_application"
```

---

## 9. ai/models.yaml — выбор моделей

```yaml
schema_version: "1.0"

# Маппинг ролей агентов на конкретные модели
role_to_model:
  document_intake:
    model: "gemma-4-26b-moe"
    fallback: "qwen-3-5-vl-32b"
    timeout_seconds: 60
    
  reconciliation:
    model: "gemma-4-31b"
    fallback: "qwen-3-5-27b"
    timeout_seconds: 30
    
  ubo_tracing:
    model: "gemma-4-31b-thinking"
    fallback: "qwen-3-5-27b-thinking"
    timeout_seconds: 120  # thinking режим медленнее
    
  conversational:
    model: "t-pro-7b"
    fallback: "vikhr-9b"
    timeout_seconds: 10  # клиентский чат — нужна скорость
    
  compliance_assistant:
    model: "gemma-4-31b"
    fallback: "qwen-3-5-27b"
    timeout_seconds: 60

# Глобальные настройки LLM Gateway
llm_gateway:
  rate_limit_per_application: 50  # max вызовов на одну заявку
  rate_limit_per_minute: 1000
  cost_alert_threshold_rub: 1000  # алерт, если на одну заявку потрачено >1000 руб.
  
# Гардрейлы (нельзя ослаблять)
guardrails:
  pii_filter: "always_on"
  output_safety_check: "always_on"
  audit_logging: "always_on"
```

---

## 10. sla.yaml — целевые SLA

```yaml
schema_version: "1.0"

sla_targets:
  # Время до резервного счёта (от подтверждения личности)
  account_reservation:
    ip: "PT5M"        # 5 минут
    llc_low_risk: "PT15M"
    llc_other: "PT2H"
    jsc: "PT1D"
    
  # Время до финального решения
  decision:
    ip: "PT30M"
    llc_low_risk: "PT4H"
    llc_other: "P1D"
    jsc: "P3D"
    
  # Время до открытия счёта в АБС после одобрения
  abs_account_opening: "PT15M"
  
  # SLA на ответ на вопросы клиента в чате
  chat_first_response: "PT30S"
  chat_human_escalation_response: "PT5M"

# Что делать при нарушении SLA
on_breach:
  account_reservation_breached:
    notify: ["compliance_officer", "support"]
    auto_action: "escalate_to_human"
    
  decision_breached:
    notify: ["compliance_officer"]
    auto_action: "alert_only"
```

---

## 11. branding/theme.json

```json
{
  "schema_version": "1.0",
  "colors": {
    "primary": "#EF3124",
    "secondary": "#1B1B1B",
    "accent": "#F4F4F4",
    "background": "#FFFFFF",
    "text_primary": "#1B1B1B",
    "text_secondary": "#666666"
  },
  "typography": {
    "heading_font": "AlfaBank Sans",
    "body_font": "AlfaBank Sans",
    "fallback": "Inter, sans-serif"
  },
  "logos": {
    "main": "vault://kv/tenants/alfa-bank/logo-main.svg",
    "compact": "vault://kv/tenants/alfa-bank/logo-compact.svg",
    "favicon": "vault://kv/tenants/alfa-bank/favicon.ico"
  },
  "domain": {
    "production": "online.alfa-bank.ru",
    "trial": "online-trial.alfa-bank.ru"
  },
  "email": {
    "from_address": "noreply@alfa-bank.ru",
    "from_name": "Альфа-Банк"
  }
}
```

---

## 12. branding/content.yaml — тексты

```yaml
schema_version: "1.0"
language: "ru"

# Тексты email-уведомлений
emails:
  application_submitted:
    subject: "Заявка на открытие счёта принята"
    template: "templates/application_submitted.html"
    
  documents_requested:
    subject: "Требуются дополнительные документы"
    template: "templates/documents_requested.html"
    
  account_opened:
    subject: "Счёт успешно открыт"
    template: "templates/account_opened.html"
    
# Тексты, показываемые в UI (могут переопределять дефолты)
ui_strings:
  welcome_message: "Добро пожаловать в Альфа-Банк! Откроем счёт за 15 минут."
  
  cta:
    start_application: "Подать заявку"
    upload_documents: "Загрузить документы"
    
# Стандартные ответы Conversational Agent
chat_responses:
  greeting: "Здравствуйте! Я помогу вам оформить счёт. Какой у вас вопрос?"
  out_of_scope: "Этот вопрос лучше задать оператору. Хотите, я переведу диалог?"
  
# Тексты ошибок (показываются клиенту)
error_messages:
  document_too_large: "Файл слишком большой. Пожалуйста, загрузите файл размером до 50 МБ."
  document_unreadable: "Не удалось распознать документ. Попробуйте загрузить более качественный скан."
```

---

## 13. Workflow конфигурации в банке

### 13.1 Кто и как редактирует

| Кто | Что редактирует | Через что |
|---|---|---|
| Бизнес-аналитик банка | Тексты, SLA, риск-политики | Web UI Admin Panel |
| Tech-lead банка | Интеграции, модели, infrastructure | YAML в Git + PR |
| Комплаенс банка | Risk rules, blocked okveds, retention | Web UI с обязательным аппрувом второго лица |
| Наш Customer Success | Помогает с миграцией, troubleshooting | YAML в Git + PR |

### 13.2 Жизненный цикл изменений

```
[Editor предлагает изменение]
    ↓
[JSON Schema validation]  ← блокируем при ошибке
    ↓
[Бизнес-валидация] (например, не убираем обязательную проверку Росфинмона)
    ↓
[Dry-run на staging] (не применяем, но проверяем)
    ↓
[PR / approval flow]
    ↓
[ArgoCD apply на staging] → smoke tests
    ↓
[Manual promote на prod]
    ↓
[Audit-запись о применении]
```

### 13.3 Откат

При проблемах после применения:

- Git revert + ArgoCD автоматически откатит
- Время отката: <5 минут
- При критическом баге — emergency rollback через CLI (с обязательным post-mortem)

---

## 14. Default vs override

Платформа поставляется с **дефолтной конфигурацией**, оптимизированной для среднего банка. Тенант может переопределять параметры только в рамках разрешённых:

- **Read-only** для тенанта: ядерные правила безопасности, обязательные регуляторные проверки, рамки SLA
- **Read-write** в пределах политики: тексты, тарифы, тон чата, дополнительные правила, ОКВЭДы
- **Read-write** свободно: брендинг, шаблоны писем, контактные лица

Каждое поле в schema помечено: `mutability: "readonly" | "policy_bounded" | "free"`.

---

## 15. Валидация

Каждый файл валидируется через несколько слоёв:

1. **JSON Schema** — структура и типы
2. **Cross-file validation** — например, ОКВЭД из workflows должен быть в blocked_okveds, если упоминается
3. **Business rules** — например, нельзя установить SLA меньше технологического минимума
4. **Reachability check** — все referenced (ABS, models) должны быть доступны
5. **Dry-run** — конфигурация прогоняется через тестовый набор сценариев

CI-пайплайн отбрасывает PR, если хотя бы один слой не прошёл.

---

## 16. Мониторинг конфигурации

В production-мониторинге для каждого тенанта:

- Текущая версия каждой конфигурации (для diagnostics)
- Время последнего изменения каждого файла
- Кто внёс последнее изменение
- Сравнение с baseline (если конфигурация сильно ушла от типовой — алерт нашему CS)
- Отсутствие критичных параметров (если убрали обязательное — заблокируем применение)

---

**Owner:** Тех-лид платформы. Schemas конфигов — в `packages/tenant-config-schema/` с обязательным версионированием.
