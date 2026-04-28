# demo-bank — пример заполненной tenant-конфигурации

Синтетический банк-тенант для демонстраций, e2e-тестов и регрессий
платформы AIbank.  **Все данные вымышленные**, не используются для
реальной эксплуатации.

## Состав

| Файл                                    | Что описывает                                    |
|-----------------------------------------|--------------------------------------------------|
| `tenant.yaml`                           | Идентификация, контракт, deployment, контакты    |
| `sla.yaml`                              | SLA для ИП / ООО / АО, эскалации                 |
| `ai/models.yaml`                        | Маппинг ролей агентов на модели (per ADR-0011)   |
| `integrations/abs.yaml`                 | Интеграция с АБС ЦФТ через mTLS, секреты в Vault |
| `risk-policy/rules.yaml`                | 7 риск-правил                                    |
| `risk-policy/thresholds.yaml`           | Скоринговые пороги                               |
| `risk-policy/blocked-okveds.yaml`       | 10 запрещённых ОКВЭД                             |

## Валидация

Конфиг проходит JSON-Schema валидацию из
`packages/tenant-config-schema/schemas/`:

```bash
python3 packages/tenant-config-schema/validate.py configs/tenants/demo-bank
# → OK: 7 file(s) validated under configs/tenants/demo-bank
```

или через CLI:

```bash
tenant-cli config-validate --path configs/tenants/demo-bank --verbose
```

## Применение

```bash
# 1. Зарегистрировать тенанта (создаёт строку в platform.tenants и схему tnt_demo_bank).
#    NB: id транслитерируется в имя схемы — допустимы [a-z0-9_]; здесь
#    используется явный ASCII-id "demo_bank".  Регистр и тире не подходят
#    для имён Postgres-схем, поэтому при создании используется underscore.
tenant-cli create \
  --id demo_bank \
  --name "Демо банк (для пилотов)" \
  --bik 044525974 \
  --inn 7700000000 \
  --deployment-mode saas

# 2. Применить миграции схемы тенанта
db-migrator tenant \
  --tenant-id demo_bank \
  --service tenant-service \
  --source services/tenant-service/migrations/tenant

# 3. Загрузить YAML-конфигурацию в tenant-service
tenant-cli config-apply --id demo_bank --path configs/tenants/demo-bank
```

> Идентификатор каталога (`demo-bank`) и идентификатор тенанта в
> Postgres (`demo_bank`) различаются: в YAML мы можем использовать
> kebab-case (соответствует regex'у в `tenant.json`), а Postgres-схема
> формируется из id с whitelist `^[a-z][a-z0-9_]{1,31}$`.  Если хочется,
> чтобы оба совпадали, используйте `demo_bank` в обоих местах.

## TODO

- Когда появится `branding/`, добавить `branding/theme.json` и
  `branding/content.yaml` для брендинга UI.
- Заменить заглушечные синтетические контакты при подготовке к реальной
  пилотной интеграции.
