# Tenant Configuration Template

Шаблон конфигурации нового банка-клиента.

Чтобы добавить нового тенанта:

1. Скопируйте папку `_template` в `configs/tenants/<tenant-id>/`
2. Заполните все файлы согласно [docs/tenant-configuration.md](../../../docs/tenant-configuration.md)
3. Откройте PR
4. Пройдите автоматическую валидацию (JSON Schema + бизнес-правила)
5. После аппрува — применение через ArgoCD

> Реальное содержимое шаблона будет добавлено в Фазе 1 разработки.
