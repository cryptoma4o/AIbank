# AIbank Codespace

Cloud-окружение для разработки и demo через GitHub Codespaces.

## Как запустить

1. Открой https://github.com/cryptoma4o/AIbank
2. Зелёная кнопка **Code** → вкладка **Codespaces** → **Create codespace on main**
3. Подожди ~3-5 минут пока image билдится и `setup.sh` отрабатывает
4. Откроется VS Code в браузере (или локальный VS Code если включил `Open in VS Code`)

## Что внутри

| Tool | Версия |
|------|--------|
| Go | 1.22 |
| Node | 20 |
| Python | 3.12 |
| Docker (DinD) | latest moby |
| golangci-lint | 1.59.0 |

## Hardware spec (host requirements)

- 4 vCPU / 8 GB RAM / 32 GB SSD
- Это бесплатно для personal account первые 60 часов в месяц

## Запуск платформы внутри Codespace

```bash
make up                  # docker-compose стек (~5 мин на cold start)
make migrate-platform    # миграции БД
make smoke               # e2e smoke
./demo/run-demo.sh       # 3 demo-сценария
```

## Frontend dev-server (отдельные терминалы)

```bash
# Terminal 1
cd apps/web-onboarding && npm run dev
# → доступен на forwarded порту 3000 (URL появится в notification)

# Terminal 2
cd apps/web-admin && PORT=3001 npm run dev
# → forwarded :3001
```

## Forwarded ports

Codespace автоматически форвардит порты обратно в твой локальный
браузер. URL вида `https://<codespace-name>-3000.app.github.dev` —
это твой dev-server.

Все ключевые порты (см. devcontainer.json):
- **3000, 3001** — frontend
- **8000** — api-gateway
- **8080-8092** — backend сервисы
- **8100-8106** — AI агенты (llm-gateway + 6 agents)
- **8201-8204** — ext-* интеграции
- **9001-9003** — ABS-адаптеры
- **5432** — PostgreSQL
- **6379** — Redis

## Что НЕ работает в Codespace

- **Реальные ext-API**: ФНС / Росфинмон / СПАРК — synthetic режим
  (как и локально)
- **GPU для vLLM**: Codespace без GPU, llm-gateway всегда в mock-mode
  (`LLM_GATEWAY_FORCE_MOCK=1`)
- **УКЭП**: stub (как и локально)

## Stop / restart

- Закрытие вкладки браузера НЕ останавливает Codespace
- Quota: автоматическое отключение через 30 минут idle (configurable)
- Полностью удалить: github.com/codespaces → выбрать → Delete

## Troubleshooting

**`make up` падает на pull image** — иногда Docker registry медленный.
Повторить через минуту. Pre-built images можно положить в GitHub
Container Registry для ускорения.

**Out of disk** — удалить published outbox rows / docker images:
```bash
docker system prune -a
```

**Slow build** — `prebuild` для devcontainer (платная фича Codespaces).
Сейчас не настроено — раз cold start ~5 мин, raz dev session.
