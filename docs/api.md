# API

Контракт — [`backend/api/openapi.yaml`](../backend/api/openapi.yaml), базовый путь `/api/v1`.
Ошибки: `{"error":{"code":"...","message":"...","details":{}}}`. Без `Authorization` → 401,
не та роль → 403, чужой ресурс → 404, эндпоинты релиза 2.0 → 501.

Разделы ниже дополняются по мере реализации вех.
