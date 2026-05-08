<!-- Parent: AGENTS.md -->
<!-- Generated: 2026-05-06 -->
<!-- Status: Draft (этап 1 «модель данных») -->

# Спецификация формы онбординга — поля и форматы по этапам

Этот документ — единая ТЗ-спецификация для **формы открытия счёта в
web-onboarding** (apps/web-onboarding) и обслуживающей доменной модели
(packages/domain-model). Каждый этап — отдельный шаг wizard'а во фронте +
соответствующее расширение схемы. Цель — провести юрлицо от ввода ИНН до
открытого счёта в соответствии с 115-ФЗ, 375-П, 152-ФЗ и 639-П.

Документ читается параллельно с:

- [docs/domain-model.md](domain-model.md) — высокоуровневая модель (Application, Person, LegalEntity, UBOGraph, Document, RiskAssessment, Decision, Account)
- [packages/domain-model/schema.json](../packages/domain-model/schema.json) — каноничная JSON Schema, источник codegen для Go/TS/Python
- [docs/compliance-map.md](compliance-map.md) — какое поле какому регуляторному требованию отвечает

Поэтапная разбивка фронта совпадает со state machine `Application`:
draft → identifying → collecting_documents → validating → risk_assessing
→ approved/declined → opening_account → account_opened.

---

## Этап 1. Предварительный скоринг

**Что делает клиент** (минимум для запуска проверок):

| Поле | Формат | Источник |
|------|--------|----------|
| `inn` | 10 цифр (юрлицо) | ввод |
| `ogrn` | 13 цифр | ввод |
| `short_name` | строка (для сверки) | ввод |

**Что подтягивается автоматически**:

| Источник | Получаем |
|----------|----------|
| ЕГРЮЛ API ФНС | статус, дата регистрации, адрес, ЕИО, учредители |
| ЗСК ЦБ | светофор (зелёный/жёлтый/красный) + дата присвоения |
| 639-П (реестр отказников) | есть/нет + причина |
| Санкционные списки (ОФАК SDN, EU CFSP, UK HMT, СДН РФ) | match score |
| Росфинмониторинг (террористы/экстремисты) | есть/нет |

---

## Этап 2. Анкета юрлица

**Идентификация:**

| Поле | Формат |
|------|--------|
| `full_name` | строка ≤500 симв. |
| `short_name` | строка |
| `opf_code` | код ОКОПФ (например `12300` = ООО) |
| `inn` | 10 цифр |
| `ogrn` | 13 цифр |
| `kpp` | 9 цифр |
| `registration_date` | дата DD.MM.YYYY |
| `registration_authority` | наименование рег. органа |
| `authorized_capital` | число + валюта (ISO 4217) |

**Адреса** (3 структурированных блока — юр., факт., почтовый):

`postal_code`, `region_code` (ФИАС), `city`, `street`, `building`, `office`,
`country_code` (ISO 3166).
Чекбокс «совпадает с юридическим» для факт./почтового.

**Деятельность:**

- `okved_main` — формат `XX.XX.XX`
- `okved_additional[]` — массив дополнительных кодов
- `licenses[]` — `{number, issue_date, expiry_date, issuer, activity_type}`
- `sro_membership[]` — `{name, reg_number, join_date}`

**Контакты:**

`phone` (`+7XXXXXXXXXX`), `email`, `website`.

**Финансовые показатели:**

- `employees_count` — число
- `revenue_last_year` — число + валюта
- `tax_regime` — справочник (ОСН / УСН-доходы / УСН-доходы-расходы / ПСН / ЕСХН / НПД)

---

## Этап 3. Сведения о деятельности (для AML)

**Описание бизнеса:**

- `business_description` — свободный текст (что именно делаете)
- `business_category` — справочник риск-категорий по ОКВЭД (низкий/средний/высокий)

**Контрагенты** (top-5 поставщиков и top-5 покупателей):

`name`, `inn`, `country`, `share_percent`, `relationship_type` (постоянный/разовый).

**Операционная модель:**

- `geography[]` — массив стран операций (ISO 3166)
- `monthly_turnover_planned` — число + валюта
- `annual_turnover_planned` — число + валюта
- `cash_share_percent` — доля наличных (0–100)
- `foreign_economic_activity` — boolean
- `foreign_countries[]` — если ВЭД=true
- `currency_operations[]` — валюты планируемых операций (ISO 4217)

**Источник средств:**

- `funds_source_category` — справочник (выручка / учредительский взнос / займ / инвестиции / другое)
- `funds_source_description` — свободный текст с обоснованием

---

## Этап 4. ЕИО и представители

По каждому лицу — отдельная запись.

**Персональные данные:**

`last_name`, `first_name`, `middle_name`, `birth_date`, `birth_place`,
`citizenship` (ISO 3166, массив для двойного), `inn` (12 цифр), `snils` (`XXX-XXX-XXX XX`).

**Документ, удостоверяющий личность:**

- `doc_type` — справочник (паспорт РФ / загранпаспорт / нац. паспорт иностранца / удостоверение беженца)
- `doc_series`, `doc_number`, `doc_issue_date`, `doc_expiry_date`
- `doc_issued_by` — кем выдан (текст)
- `doc_department_code` — код подразделения (для РФ)

**Адреса:** `registration_address`, `actual_address` (структурированные).

**Полномочия:**

- `position` — должность
- `authority_basis` — справочник (устав / протокол / доверенность / приказ)
- `authority_doc_number`, `authority_doc_date`
- `signature_sample` — загрузка изображения подписи

**Для иностранных граждан** (дополнительно):

- `migration_card_number`, `migration_card_dates`
- `residence_doc_type` (РВП/ВНЖ/виза), `residence_doc_number`, `residence_dates`

**Декларации:**

- `is_pdl` — boolean (публичное должностное лицо)
- `pdl_category` — иностранное / российское / международной организации
- `pdl_position`, `pdl_relation` (сам / родственник / представитель)

---

## Этап 5. Бенефициарные владельцы (УБО)

Все поля **этапа 4** + дополнительно:

- `direct_ownership_percent` — прямая доля
- `indirect_ownership_percent` — косвенная доля
- `control_basis` — справочник (доля капитала / договор / иное право)
- `ownership_chain[]` — массив звеньев: `{level, entity_name, entity_inn_or_reg_number, country, share_percent}`
- `chain_diagram` — загрузка файла со схемой
- `is_sole_beneficiary` — boolean

**Налоговое резидентство (FATCA/CRS):**

- `tax_residency_countries[]` — массив стран
- `tin_per_country[]` — TIN по каждой стране
- `us_person` — boolean (FATCA)
- `w8_w9_form` — загрузка формы (если применимо)

**Если УБО не определены:**

- `no_ubo_reason` — обоснование
- `eio_as_ubo_confirmation` — boolean

---

## Этап 6. Документы (загрузка файлов)

Каждый документ — `{file, doc_type, issue_date, expiry_date, signed_by}`:

- `charter` — устав (PDF, актуальная редакция)
- `egrul_record_sheet` — лист записи ЕГРЮЛ
- `eio_appointment_protocol` — решение/протокол о назначении
- `eio_order` — приказ о вступлении в должность
- `signature_card` — карточка с образцами подписей и оттиска печати
- `licenses[]` — лицензии и разрешения
- `address_confirmation` — договор аренды / свидетельство о собственности
- `financial_statements` — отчётность за последний год (баланс + ОФР)
- `tax_declaration` — последняя налоговая декларация
- `tax_clearance_certificate` — справка ФНС об отсутствии задолженности (опц.)
- `power_of_attorney[]` — доверенности на представителей

**Технические требования к файлам:** PDF/A или сканы (JPG/PNG до 10 МБ),
УКЭП или собственноручная подпись с печатью, читаемость всех страниц.

---

## Этап 7. AML-проверки (внутренние данные банка)

Клиент сюда ничего не вводит — генерируются данные для скоринга:

- `sanctions_screening_result` — match / partial / no match по каждому лицу × каждый список
- `pep_screening_result` — статус по каждому лицу
- `adverse_media_hits[]` — массив негативных публикаций
- `okved_consistency_score` — соответствие ОКВЭД и заявленной деятельности (0–100)
- `turnover_realism_score` — реалистичность планируемых оборотов vs ОКВЭД и численность
- `device_fingerprint`, `ip_address`, `geolocation` — фрод-сигналы при подаче

---

## Этап 8. Риск-скоринг

Итоговые расчётные поля:

- `risk_score` — числовой балл (0–100)
- `risk_level` — справочник (низкий / средний / высокий)
- `risk_factors[]` — массив сработавших факторов с весами
- `monitoring_profile` — справочник профилей мониторинга
- `review_frequency_months` — 12 / 6 / 3
- `transaction_limits` — JSON с лимитами по типам операций

---

## Этап 9. Открытие счёта

**Параметры счёта:**

- `account_type` — справочник (расчётный / специальный / валютный / эскроу / номинальный)
- `currency` — ISO 4217
- `tariff_plan` — справочник тарифов

**Согласия и подписки:**

- `agreement_acceptance` — boolean + timestamp
- `dbo_agreement` — согласие на ДБО + выбор канала (интернет-банк / мобильный / API)
- `edo_agreement` — согласие на ЭДО
- `personal_data_consent` — согласие на обработку ПДн
- `signing_method` — УКЭП / СМС-код / собственноручно
- `ukep_certificate` — серийный номер сертификата (если УКЭП)

**Реквизиты, генерируемые банком:**

- `account_number` — 20 цифр
- `bik`, `correspondent_account`
- `account_open_date`

---

## Этап 10. Данные для постоянного мониторинга

Закладываются на онбординге:

- `next_review_date` — дата следующего пересмотра
- `monitoring_rules[]` — какие правила 375-П применяются
- `kyc_refresh_triggers[]` — события для перезапуска KYC (смена ЕИО, превышение лимитов и т.д.)
- `notification_settings` — куда уведомлять о подозрительных операциях
