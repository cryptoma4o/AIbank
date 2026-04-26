# Доменная модель

Каноническая модель данных платформы. Это **контракт между всеми сервисами и UI** — менять можно только через ADR и согласование с архитектором.

Модель хранится в `packages/domain-model/` и автогенерируется в TypeScript, Go и Python из единого источника (схемы в JSON Schema + OpenAPI).

---

## 1. Принципы

- **Богатые ID-схемы.** Каждая сущность имеет ID-формат с префиксом типа: `app_…`, `client_…`, `doc_…`. Это сильно упрощает отладку и логи.
- **Immutable-события + mutable-проекции.** Все изменения — события, состояние — производное.
- **Отсутствие cascade delete.** Сущности не удаляются физически, только помечаются `archived_at`. Хранение 5 лет минимум.
- **Все деньги — в копейках, целые числа.** Никогда `float`.
- **Все даты — UTC + ISO 8601** (`2026-04-26T14:30:00Z`). Local time только на UI.
- **Multi-tenant.** Каждая сущность принадлежит одному `tenant_id`. Cross-tenant связи запрещены архитектурно.

---

## 2. Главные сущности

### 2.1 Tenant (Банк)

Корневая сущность. Каждый банк — отдельный тенант со своей конфигурацией, изолированными данными, отдельной базой.

```yaml
Tenant:
  id: string              # tnt_alfa_bank
  name: string            # "Альфа-Банк"
  legal_name: string      # "АО Альфа-Банк"
  bik: string             # 044525593
  inn: string             # 7728168971
  status: enum            # active | suspended | archived
  deployment_mode: enum   # saas | on_prem
  created_at: timestamp
  contract: 
    contract_id: string
    start_date: date
    end_date: date
    pricing_tier: string
  configuration_version: string  # ссылка на текущую версию config
  contacts:
    technical_lead: { name, email }
    compliance_officer: { name, email }
    business_owner: { name, email }
```

### 2.2 Application (Заявка на онбординг)

Главная workflow-сущность. Один Application = одна заявка от начала до открытия счёта (или отказа).

```yaml
Application:
  id: string                # app_01HQ8X4Z...
  tenant_id: string
  external_id: string?      # если заявка пришла из CRM банка
  state: ApplicationState   # см. state machine ниже
  legal_entity_type: enum   # IP | LLC | JSC | NPF
  channel: enum             # web | mobile | courier | branch
  
  applicant_id: string      # ссылка на Person — кто подал заявку
  legal_entity_id: string?  # ссылка на LegalEntity (создаётся не сразу)
  
  product_codes: [string]   # ["current_rub", "current_usd", "online_banking"]
  
  workflow_id: string       # Temporal workflow ID
  
  risk_assessment_id: string?  # ссылка на RiskAssessment (заполняется в процессе)
  decision_id: string?         # ссылка на Decision (заполняется в финале)
  account_ids: [string]        # ссылки на созданные Account (после одобрения)
  
  metadata:
    created_at: timestamp
    updated_at: timestamp
    completed_at: timestamp?
    archived_at: timestamp?
    sla_target: timestamp?    # обещанный клиенту срок
    referral_source: string?  # откуда пришёл клиент
```

#### Application State Machine

```
[draft]
    ↓ (applicant submits)
[identifying]
    ↓ (identity verified)
[collecting_documents]
    ↓ (all docs uploaded)
[validating]
    ↓ (extraction + reconciliation done)
    ├─→ [waiting_for_client] (если есть вопросы) ─→ обратно в [validating]
    └─→ [risk_assessing]
            ↓
            ├─→ [auto_approved] (зелёная зона)
            ├─→ [manual_review] (нужен оператор)
            │       ↓
            │       ├─→ [approved]
            │       ├─→ [approved_with_edd] (с расширенной проверкой)
            │       └─→ [declined]
            └─→ [requires_more_info] (от клиента, обратно в waiting)
            
[approved] | [approved_with_edd]
    ↓
[opening_account] (АБС)
    ↓
[account_opened] — терминальное успех
[declined] — терминальный отказ  
[abandoned] — клиент бросил процесс на 30+ дней
```

Переходы между состояниями — события в audit log. Откат недопустим (переход назад = создание нового Application).

### 2.3 Person (Физлицо)

Любое физлицо в системе: заявитель, директор, бухгалтер, учредитель-физлицо, УБО, контактное лицо.

```yaml
Person:
  id: string                # per_01HQ8X4Z...
  tenant_id: string
  
  identification:
    full_name: string       # "Иванов Иван Иванович"
    last_name: string
    first_name: string
    middle_name: string?
    birth_date: date
    citizenship: string     # ISO 3166-1 alpha-2: "RU", "BY"
    
  documents:
    inn: string?            # 12 знаков для физлица
    snils: string?
    passport:
      series: string
      number: string
      issued_by: string
      issued_at: date
      issued_code: string
    foreign_passport:        # для нерезидентов
      number: string
      issued_by: string
    
  contacts:
    email: string?
    phone: string?
    registration_address: Address?
    actual_address: Address?
    
  flags:
    is_pep: boolean         # politically exposed person
    is_sanctioned: boolean
    is_fpd: boolean         # foreign public official
    
  created_at: timestamp
  updated_at: timestamp
```

### 2.4 LegalEntity (Юрлицо)

Центральная сущность для ООО/АО. Для ИП используется чуть упрощённая модель (`IndividualEntrepreneur` extends Person).

```yaml
LegalEntity:
  id: string                # le_01HQ8X4Z...
  tenant_id: string
  type: enum                # LLC | JSC | NPF | NCO | OTHER
  
  registry_data:            # данные из ЕГРЮЛ
    inn: string             # 10 знаков
    ogrn: string            # 13 знаков
    kpp: string             # 9 знаков
    full_name: string       # "Общество с ограниченной ответственностью «Ромашка»"
    short_name: string      # "ООО «Ромашка»"
    registration_date: date
    registration_authority: string
    
  classification:
    okveds:
      main: { code: string, name: string }
      additional: [{ code: string, name: string }]
    okfs: string            # форма собственности
    okopf: string           # организационно-правовая форма
    
  status:
    is_active: boolean      # есть ли запись о ликвидации
    is_in_bankruptcy: boolean
    is_in_liquidation: boolean
    last_egrul_check: timestamp
    
  capital:
    declared: integer       # уставной капитал в копейках
    paid: integer
    currency: string        # обычно "RUB"
    
  addresses:
    legal_address: Address  # юридический
    actual_address: Address?  # фактический, если отличается
    
  officers:
    director: PersonReference   # ЕИО
    other_signatories: [PersonReference]
    accountants: [PersonReference]
    
  ownership:
    founders: [Founder]     # из ЕГРЮЛ
    ubo_graph_id: string?   # ссылка на UBOGraph, заполняется агентом
    ubos: [UBOReference]
    
  flags:
    is_resident: boolean
    has_foreign_owners: boolean
    has_state_participation: boolean
    is_strategic_enterprise: boolean   # для 57-ФЗ
    
  data_quality:
    completeness: float     # 0..1
    last_full_check: timestamp
    
  created_at: timestamp
  updated_at: timestamp


Founder:                    # учредитель в выписке ЕГРЮЛ
  type: enum                # PERSON | LEGAL_ENTITY_RU | LEGAL_ENTITY_FOREIGN | STATE
  reference: 
    person_id: string?      # если type = PERSON
    legal_entity_id: string?# если type = LEGAL_ENTITY_RU
    foreign_data: { name, country, registration_number }?  # если FOREIGN
  share_percent: decimal    # 0..100
  share_nominal: integer?   # в копейках
  voting_rights_percent: decimal?  # для АО может отличаться от share
  is_active: boolean        # текущий ли учредитель
```

### 2.5 UBOGraph (Граф владения)

Отдельная сущность, потому что граф может пересчитываться (новые документы, изменения в ЕГРЮЛ).

```yaml
UBOGraph:
  id: string
  tenant_id: string
  legal_entity_id: string   # корневое юрлицо
  version: integer
  
  nodes: [UBONode]
  edges: [UBOEdge]
  
  ubos: [UBO]               # вычисленные конечные бенефициары
  
  computation:
    computed_at: timestamp
    computed_by: string     # "agent-ubo-tracing v1.3"
    confidence: float       # 0..1
    unresolved_branches: [UnresolvedBranch]
    

UBONode:
  id: string                # node-1, node-2 в рамках графа
  type: enum                # PERSON | LEGAL_ENTITY_RU | LEGAL_ENTITY_FOREIGN
  reference:
    person_id: string?
    legal_entity_id: string?
    foreign_data: object?
    

UBOEdge:
  from_node: string
  to_node: string
  type: enum                # OWNERSHIP | CONTROL | SIGNATORY
  share_percent: decimal?
  voting_percent: decimal?
  

UBO:
  person_id: string
  effective_share_percent: decimal
  control_basis: enum       # OWNERSHIP | VOTING_CONTROL | EIO_APPOINTMENT_RIGHT | OTHER
  paths: [[string]]         # пути из графа, объясняющие долю
  is_confirmed: boolean     # подтверждён клиентом или комплаенс-офицером
```

### 2.6 Document (Документ)

Любой документ в системе — загруженный клиентом, полученный из внешнего источника, сгенерированный платформой.

```yaml
Document:
  id: string                # doc_01HQ8X4Z...
  tenant_id: string
  application_id: string?
  
  type: enum                # PASSPORT | CHARTER | PROTOCOL | AGREEMENT | EGRUL_EXTRACT | ...
  subtype: string?          # уточнение, например "PROTOCOL_DIRECTOR_APPOINTMENT"
  
  source:
    type: enum              # CLIENT_UPLOAD | EGRUL | ESIA | GENERATED | COURIER
    actor_id: string?       # кто загрузил/получил
    
  files:
    - file_id: string       # file_…
      filename: string
      mime_type: string
      size_bytes: integer
      sha256: string
      storage_path: string  # s3://bucket/...
      
  state: enum               # uploaded | parsed | validated | rejected | archived
  
  parsed_data:              # результат Document Intake Agent
    schema_version: string
    fields: object
    extraction_quality: object  # confidence по полям
    parsed_at: timestamp
    parsed_by: string       # модель + версия
    
  validation:
    is_valid: boolean
    issues: [string]
    validated_at: timestamp
    validated_by: string?   # human_id или agent_name
    
  signatures:
    - signer_id: string     # person_id
      signature_type: enum  # UKEP | PEP | HANDWRITTEN_SCAN | NOTARIZED
      signed_at: timestamp
      certificate_serial: string?
      verification_status: enum  # valid | invalid | expired
      
  retention:
    expires_at: timestamp   # min 5 лет от created_at
    legal_hold: boolean     # запрет на удаление (судебные дела)
  
  created_at: timestamp
  updated_at: timestamp
```

### 2.7 RiskAssessment (Риск-оценка)

```yaml
RiskAssessment:
  id: string
  tenant_id: string
  application_id: string
  legal_entity_id: string?
  
  score: decimal            # 0..1
  category: enum            # LOW | MEDIUM | HIGH
  
  model:
    name: string            # "catboost-onboarding-v1.5"
    version: string
    computed_at: timestamp
    
  factors:                  # SHAP-values для объяснимости
    - name: string          # "company_age"
      value: any            # "2 years"
      weight: decimal       # -0.15
      direction: enum       # INCREASES_RISK | DECREASES_RISK
      
  rules_triggered:          # жёсткие правила, сработавшие независимо от модели
    - rule_id: string       # "OKVED_BLOCKED"
      severity: enum        # WARNING | BLOCKING
      description: string
      
  screening_results:
    rosfinmon: ScreeningResult
    fssp: FsspResult
    arbitrage: ArbitrageResult
    
  explanation:              # текст от LLM, на основе factors
    text: string
    generated_by: string    # модель + версия
    
  recommendation: enum      # AUTO_APPROVE | MANUAL_REVIEW | DECLINE_RECOMMENDED
  
  reviewed_by: string?      # человек, если был manual review
  review_decision: enum?    # match | override
  review_comment: string?
```

### 2.8 Decision (Финальное решение)

```yaml
Decision:
  id: string
  tenant_id: string
  application_id: string
  
  decision: enum            # APPROVED | APPROVED_WITH_EDD | DECLINED | ESCALATED
  decided_by:
    actor_type: enum        # AUTO | HUMAN
    actor_id: string        # system или operator_id
  decided_at: timestamp
  
  reasoning: string         # обязательно для всех решений, особенно DECLINED
  
  conditions:               # для APPROVED_WITH_EDD — какие условия
    - type: string
      description: string
      review_period_days: integer?
      
  evidence:                 # ссылки на всё, что повлияло на решение
    risk_assessment_id: string
    documents: [string]
    audit_event_ids: [string]
    
  client_notification:
    notified_at: timestamp?
    method: enum            # EMAIL | SMS | PUSH | NONE
    text_shown: string      # что именно показали клиенту
    declined_reason_disclosed: boolean  # для DECLINED — раскрывали ли причину
```

### 2.9 Account (Счёт)

```yaml
Account:
  id: string                # acc_01HQ8X4Z...
  tenant_id: string
  application_id: string
  legal_entity_id: string
  
  number: string            # 20-значный номер счёта по плану счетов
  bik: string               # БИК банка
  correspondent_account: string
  
  type: enum                # CURRENT | DEPOSIT | LOAN
  currency: string          # ISO 4217: RUB, USD, EUR, CNY
  
  state: enum               # reserved | activating | active | suspended | closed
  
  abs_data:
    abs_account_id: string  # внутренний ID в АБС
    abs_type: string        # тип, как АБС называет
    
  reservation:
    reserved_at: timestamp
    expires_at: timestamp?  # резерв обычно на 30 дней
    
  activation:
    activated_at: timestamp?
    activation_decision_id: string  # ссылка на Decision
    
  signature_card_id: string?  # карточка с образцами подписей
  
  closure:
    closed_at: timestamp?
    closure_reason: string?
    closure_initiated_by: enum  # CLIENT | BANK | LAW_REQUIREMENT
```

### 2.10 AuditEvent (Событие аудита)

Append-only, immutable. Ключевая сущность для регуляторного соответствия.

```yaml
AuditEvent:
  id: string                # ULID, отсортирован по времени
  tenant_id: string
  
  timestamp: timestamp      # время события
  recorded_at: timestamp    # время записи (могут отличаться при асинхронной записи)
  
  actor:
    type: enum              # USER | SYSTEM | AI_AGENT | EXTERNAL_API
    id: string
    metadata: object        # для AI_AGENT — модель, версия промпта
    
  action: string            # "application.submitted", "decision.approved", "document.uploaded"
  
  subject:
    type: string            # "application", "document", "decision"
    id: string
    
  data: object              # доменные данные события
  
  context:
    application_id: string?
    workflow_id: string?
    trace_id: string
    ip_address: string?     # для USER
    user_agent: string?
    
  signature: string         # криптографическая подпись записи
  previous_event_id: string?  # для chain-проверки целостности
```

---

## 3. Вспомогательные value objects

### Address

```yaml
Address:
  raw_text: string          # "г. Москва, ул. Тверская, д. 1, кв. 42"
  
  structured:               # после нормализации
    country: string         # ISO: RU
    region: string          # "Москва"
    region_code: string?    # "77"
    city: string?
    settlement: string?
    street: string?
    house: string?
    building: string?
    apartment: string?
    postal_code: string?
    
  fias_id: string?          # ID в ФИАС, если найден
  is_mass_registration: boolean  # флаг массового адреса (для ФНС-критериев)
```

### PersonReference

Лёгкая ссылка на Person, чтобы не дублировать данные.

```yaml
PersonReference:
  person_id: string
  full_name: string         # денормализовано для UI
  inn: string?
  role: string              # "DIRECTOR", "FOUNDER", "UBO"
```

### ScreeningResult

```yaml
ScreeningResult:
  source: string            # "ROSFINMON", "OFAC", "EU_SANCTIONS"
  checked_at: timestamp
  has_matches: boolean
  matches: [
    { 
      list_name: string,
      matched_field: string,
      confidence: float,
      details: object
    }
  ]
```

---

## 4. Идентификаторы

Все ID — формат `{тип}_{ULID}`. Примеры:

- `tnt_01HQ8X4Z9V6XYZK1234567890` — Tenant
- `app_01HQ8X4Z9V6XYZK1234567891` — Application
- `per_01HQ8X4Z9V6XYZK1234567892` — Person
- `le_01HQ8X4Z9V6XYZK1234567893` — LegalEntity
- `doc_01HQ8X4Z9V6XYZK1234567894` — Document
- `acc_01HQ8X4Z9V6XYZK1234567895` — Account

ULID, а не UUID, потому что:

- Сортируется по времени создания (упрощает дебаг)
- Безопасен для URL
- Длина фиксированная

---

## 5. Versioning стратегия модели

Доменная модель будет меняться. Стратегия:

- **Backward-compatible изменения** (добавление полей с дефолтами) — minor version, без миграций
- **Breaking changes** — major version, обязательно ADR + миграция данных
- **Поддерживаем N-1 версию** API в течение 6 месяцев минимум

Версия модели хранится в `packages/domain-model/version.txt` и проставляется во всех сериализованных представлениях через поле `_schema_version`.

---

## 6. Связи между сущностями

```
Tenant 1──n Application
Application 1──1 Person (applicant)
Application 0..1──1 LegalEntity (для ИП может не быть)
Application 1──n Document
Application 1──1 RiskAssessment
Application 1──1 Decision
Application 1──n Account

LegalEntity 1──1 UBOGraph
LegalEntity n──n Person (через PersonReference в officers)
UBOGraph 1──n Person (UBOs)

Document 0..n──n Person (signatures)

Account 1──1 LegalEntity
```

---

## 7. Что НЕ в доменной модели

Намеренно не включаем:

- **Финансовые транзакции** — это смежная система банка, не онбординг
- **Внутренние пользователи банка** (операторы, комплаенс) — отдельный контекст IAM
- **Маркетинговые атрибуты** — отдельная аналитическая база
- **Билинг и лимиты счёта** — это в АБС, не у нас
- **Карточки и эквайринг** — после открытия счёта, другая система

Эти границы важны: домен онбординга должен оставаться чистым.

---

**Owner:** Главный архитектор. Изменения — через PR с ADR при breaking changes.
