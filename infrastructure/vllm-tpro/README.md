# vLLM + T-Pro 32B на GPU

Готовый стек для GPU-сервера, на котором будет крутиться **T-Pro** (T-Bank's
32B RU-LLM). Используется как backend для нашего `llm-gateway`.

## Сценарии использования

### 1. SaaS staging/prod на арендованном GPU

Используется когда наш staging-сервер `206.204.106.28` не имеет GPU.
Поднимаем отдельный GPU-хост (Selectel ML / Yandex Cloud) и подключаем
gateway по сети.

### 2. On-prem у банка-клиента

Банк предоставляет собственный H100/A100, на нём разворачивается этот стек.
gateway в их Kubernetes указывает на их vLLM через `VLLM_TPRO_URL`.

## Минимальные требования

| Квантизация | VRAM | Подходящие GPU |
|-------------|------|----------------|
| Q4 (AWQ/GGUF) — рекомендовано | 22-24 GB | RTX 4090 / A100 40GB / L40S 48GB / H100 80GB |
| FP16 — максимальное качество | 64 GB | H100 80GB / 2×A100 40GB |

Прочее:
- nvidia-container-toolkit
- Docker ≥ 20.10
- SSD ≥ 60GB для кэша HF (модель + checkpoint)

## Быстрый старт

```bash
# 1. На GPU-хосте:
git clone <repo> /opt/aibank-vllm
cd /opt/aibank-vllm/infrastructure/vllm-tpro
cp .env.example .env
# (отредактируй .env при необходимости)

docker compose up -d
docker compose logs -f vllm-tpro  # ждём ~3-5 мин cold-load модели

# 2. Smoke-check:
curl http://localhost:8000/v1/models
curl -X POST http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"t-pro","messages":[{"role":"user","content":"Здравствуй"}]}'

# 3. На основном AIbank-сервере подключаем gateway:
#    В docker-compose.yml у llm-gateway:
export VLLM_TPRO_URL=http://<gpu-host>:8000
docker compose up -d --force-recreate --no-deps llm-gateway

# 4. Проверяем что gateway маршрутизирует на T-Pro:
curl -X POST http://localhost:8100/v1/chat/completions \
  -H "X-Tenant-Id: demo" \
  -H "Content-Type: application/json" \
  -d '{"model":"role:text-reasoning","messages":[{"role":"user","content":"test"}]}'
# В response: "aibank_gateway.backend": "vllm-tpro"
```

## TLS

В production GPU-хост **обязательно** за TLS-прокси:
- nginx + Let's Encrypt на публичный домен `gpu.aibank.ru:443 → :8000`
- Или mTLS между gateway и vLLM (для on-prem банков)
- Или VPN между AIbank-cloud и GPU-хостом

В .env добавить `VLLM_TPRO_URL=https://gpu.aibank.ru` (с https).

## Стоимость аренды (для прикидки)

| Провайдер | GPU | Цена / час | Месяц 24/7 |
|-----------|-----|-----------|------------|
| Selectel ML | L40S 48GB | ~150 ₽ | ~110k ₽ |
| Yandex Cloud | A100 40GB (spot) | ~80 ₽ | ~60k ₽ |
| Servers.ru | 2×L40S dedicated | — | ~100k ₽/мес |

Для on-prem банка — капекс самого банка, нас не касается.

## Переключение моделей

Поменять модель — только `.env` + перезапуск:
```bash
sed -i 's|^VLLM_MODEL_ID=.*|VLLM_MODEL_ID=Qwen/Qwen2.5-32B-Instruct-AWQ|' .env
docker compose up -d --force-recreate vllm-tpro
```

Имя в нашем llm-gateway `staging-ollama.yaml` / `default-routing.yaml` остаётся
`t-pro` — vLLM выставляет `--served-model-name t-pro`, поэтому contract
с нашим gateway не ломается.

## Мониторинг

```bash
# Метрики Prometheus (стандартные у vLLM):
curl http://localhost:8000/metrics

# GPU usage:
nvidia-smi
docker stats vllm-tpro

# Logs:
docker compose logs --tail=200 -f vllm-tpro
```

## Откат

Если что-то пошло не так — gateway сам fallback'нется на mock:
```bash
unset VLLM_TPRO_URL    # вернёт URL из default-routing.yaml (http://vllm-tpro:8000 — не резолвится → fallback)
docker compose up -d --force-recreate --no-deps llm-gateway
```
