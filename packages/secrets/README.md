# packages/secrets

Shared Go module that abstracts secret retrieval for AIbank services.

Per `docs/security-architecture.md` § 7.3 and ADR-0002, production services must
not read secrets from process env vars; they go through HashiCorp Vault. In dev
we transparently fall back to env vars to keep onboarding cheap.

## Providers

| Provider | Backend | When to use |
|---|---|---|
| `EnvProvider`     | `os.Getenv`              | Local/dev |
| `VaultProvider`   | Vault KV v2              | Prod / staging |
| `ChainedProvider` | Vault first, env fallback | Default |

## Quickstart

```go
import "github.com/aibank/platform/packages/secrets"

prov, err := secrets.BuildProvider(secrets.FactoryConfig{
    ServiceName: "identity-service",
})
if err != nil { log.Fatal(err) }

dbURL, err := prov.GetSecret(ctx, "database/url") // -> secret/data/aibank/identity-service/database#url
jwtSecret, err := prov.GetSecret(ctx, "jwt/secret")
```

## Environment variables

| Var | Default | Purpose |
|---|---|---|
| `SECRETS_BACKEND`   | `chained`  | `vault` \| `env` \| `chained` |
| `VAULT_ADDR`        | -          | Vault URL (e.g. `https://vault.svc:8200`) |
| `VAULT_TOKEN`       | -          | Vault token (prod: AppRole / Vault Agent — TODO) |
| `VAULT_NAMESPACE`   | -          | Vault Enterprise namespace |
| `VAULT_MOUNT`       | `secret`   | KV v2 mount path |
| `VAULT_PATH_PREFIX` | `aibank/<service>` | Per-service path prefix |

For `chained` backend: if `VAULT_ADDR`+`VAULT_TOKEN` are unset we skip Vault and
serve env directly — keeps `docker-compose up` workflow zero-config.

## Key naming

Keys are backend-agnostic and look like `database/url`, `jwt/secret`. The last
segment is the KV field; everything before it is the path. Single-segment keys
default the field to `value`. Env lookup uppercases and replaces `/` and `-`
with `_` (so `database/url` -> `DATABASE_URL`).

## Cache strategy

`VaultProvider` keeps an in-memory `map[key]{value,expires}` with default TTL
**5 min** (`VaultConfig.CacheTTL`). On hit and not expired, no network call.
Rotation requires `InvalidateCache()` or a service restart — Vault Agent
sidecar template-based rendering is the longer-term answer (TODO).

## Security notes (do / don't)

- **DO** treat any non-`ErrNotFound` error from Vault as fatal at startup.
- **DO NOT** log secret values or even keys at INFO level.
- **DO NOT** add Vault deps to services that don't need them — only the
  packages/services explicitly wired.

## Field-level encryption (Vault Transit)

Per `docs/security-architecture.md` § 5.2, особо чувствительные PII-поля шифруются
на уровне приложения **поверх TDE** через Vault Transit Engine. Это защищает
данные при компрометации БД (dump утёк, бэкап восстановлен на стороннем хосте,
SQL-injection с прямым SELECT и т.п.) — без действующего Vault-токена они
бесполезны.

### Когда использовать

Шифровать через Transit:

- серия и номер паспорта;
- СНИЛС;
- ИНН физлица (если хранится отдельно от индексных колонок);
- скан-копии документов (отдельный flow через `transit/encrypt` для байт);
- любые поля, попадающие под "ПДн особой категории" 152-ФЗ.

**НЕ шифровать** часто-запрашиваемые/индексируемые поля (full_name, phone),
если по ним нужен SQL `WHERE` или индекс — Transit ломает поиск. Для них
полагаемся на TDE + audit-log + access controls.

### Per-tenant key naming

Соглашение: `aibank-tenant-<tenant_id>` (см. `TenantKeyPrefix`, `TenantKeyName()`).
Каждый тенант шифруется своим Transit-ключом — это упрощает:

- **изоляцию**: компрометация ключа банка-A не затрагивает банк-B;
- **offboarding**: удаление tenant-key делает все его данные мусором (right-to-be-forgotten / расторжение договора);
- **rotation**: можно ротировать только пострадавший ключ.

Иерархия (см. § 6.2): master-key (HSM / sealed Vault) → **tenant-key** ←
этот код → data-key (внутри Transit).

### Code example (repository pattern)

```go
import "github.com/aibank/platform/packages/secrets"

type ApplicantRepo struct {
    db      *sql.DB
    transit secrets.Transit // nil-safe: dev-mode ходит plaintext
}

func (r *ApplicantRepo) Create(ctx context.Context, a *domain.Applicant) error {
    keyName := secrets.TenantKeyName(a.TenantID)
    if r.transit != nil {
        if err := r.transit.EnsureKey(ctx, keyName); err != nil {
            return err
        }
    }

    passportSeries, err := secrets.EncryptOrEmpty(ctx, r.transit, keyName, a.PassportSeries)
    if err != nil { return err }
    snils, err := secrets.EncryptOrEmpty(ctx, r.transit, keyName, a.SNILS)
    if err != nil { return err }

    _, err = r.db.ExecContext(ctx,
        `INSERT INTO applicants (id, passport_series_enc, snils_enc, ...) VALUES ($1, $2, $3, ...)`,
        a.ID, []byte(passportSeries), []byte(snils), /* ... */)
    return err
}
```

### Mock for tests

`secrets.NewMockTransit()` — детерминистический XOR-based шифратор. **Не
использовать в проде**: годится только для round-trip-тестов и dev-fixture'ов.

### TODOs

- **Key rotation**: на сегодня rewrap не автоматизирован. Нужен `transit/rewrap/<key>` workflow + миграционный батч-job, переписывающий все BYTEA-колонки под новую версию ключа (`vault:v2:...`).
- **ГОСТ-2012**: для банков, обязанных использовать сертифицированные СКЗИ
  (КриптоПро / VipNet), Vault Transit (AES-256-GCM) не подходит. Под этот
  кейс — отдельный provider, оборачивающий нативный API КриптоПро CSP / VipNet
  (см. § 6.3). Архитектурно: интерфейс `secrets.Transit` расширяется второй
  имплементацией, выбор — через tenant-конфиг.
- **Migration path для legacy plaintext rows**: пока обе колонки (TEXT и
  BYTEA) сосуществуют. После rollout'а — batch-job, читающий plaintext-колонку,
  пишущий в `_enc`, после верификации — `DROP COLUMN ... ; ALTER ... NOT NULL`
  отдельной миграцией.
- **Performance**: Transit — это сетевой round-trip на каждое поле. Для batch-сценариев использовать `transit/encrypt` с `batch_input` (TODO).

## Open TODOs

- AppRole login flow (replace static `VAULT_TOKEN`).
- Vault Agent sidecar with templated files (avoids pulling secrets in-process).
- Rotation policy + reload signal (SIGHUP) for long-lived processes.
- Field-level encryption hardening: см. секцию выше.
