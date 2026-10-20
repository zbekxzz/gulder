# CLAUDE.md — правила проекта Gulder

Полное задание — `TASK.md` (в нём проект называется Flowers; рабочее имя — **Gulder**).
Здесь только то, что должно быть в контексте на каждом шаге: §7 (Rules Engine) и §13 (безопасность).
Журнал вех — `PROGRESS.md`, решения — `docs/adr/`.

## Как работать
- Вехи M0→M9 строго по порядку (§17). Веха закрыта: `make test` и `make lint` зелёные, `PROGRESS.md` обновлён, git-тег `mN`.
- Внутри вехи — ветка на задачу (`mN/<тема>`), маленькие осмысленные коммиты, merge `--no-ff` в `main`.
- Код требования (FR-…, RM-…, AC-…) — в комментарии рядом с реализацией и в имени теста (`TestAC1_…`).
- Значимое решение → ADR `docs/adr/NNNN-название.md`.
- Данные о токсичности и аллергенах не выдумывать: только `data/plant_taxa.csv` (Приложение B); нет данных → `unknown`.
- Схема БД меняется только новой миграцией goose (с рабочим Down); `00001_init.sql` = Приложение A.
- Деньги — `domain.Money` (тиыны, int64), в БД `numeric(12,2)`. Никогда не float.
- Интеграционные тесты: `TEST_DATABASE_URL` (локальный Postgres) или testcontainers; без Docker — skip.

## Rules Engine (`backend/internal/rules`) — ядро безопасности (§7)
Чистый Go: без БД, HTTP, LLM. Вызывается из рекомендаций, ассистента, фильтров, замен и eval.
**Ни один путь (ranker, ассистент, фильтры, нумерология, замены, ручные действия) не может его обойти или снять BLOCK.**

Принципы:
1. «Не знаю» ≠ «нет ограничения». Неизвестное поле не даёт ALLOW рискованной позиции: WARN + штраф + `missing_info`.
2. «Предполагаю» = «знаю» для ограничений. Поле известно, если `confidence != unknown && value != null`.
3. Неопознанное растение, неизвестная токсичность для указанного питомца (cat/dog/horse), неизвестный состав подарка при известных аллергиях → BLOCK.
4. BLOCK окончательный.

Аллергены: `genus:<род>`, `family:<семейство>`, `pollen`, `food:<группа>[:<подгруппа>]`, `fragrance`, `latex`.
Аллергия `a` совпадает с кодом `c`, если `c == a` или `c` начинается с `a + ":"`.
Ключи растения: `genus:`, `family:`, его `AllergenCodes`, `pollen` при пыльце ≥ medium.

Цветок: F1 нет таксона/не подтверждён → BLOCK `UNVERIFIED_PLANT` · F2 аллергия ∩ ключи → BLOCK `ALLERGEN_MATCH` ·
F3 аллергии неизвестны и (семейство ∈ {asteraceae, liliaceae, amaryllidaceae, oleaceae} или пыльца ≥ medium) → WARN `ALLERGY_UNKNOWN` 0.15 ·
F4 питомец вне {cat,dog,horse} → WARN `PET_NO_DATA` 0.20 · F5 токсичность для указанного вида ≠ none (вкл. unknown) → BLOCK `PET_TOXIC` ·
F6 питомцы неизвестны, токсичность для cat/dog ≠ none → WARN `PETS_UNKNOWN` 0.35 (severe/unknown) иначе 0.15 ·
F7/F8 нелюбимый род/цвет → WARN `DISLIKED_FLOWER`/`DISLIKED_COLOR` 0. Пустой букет → BLOCK `EMPTY_COMPOSITION`.

Запах/астма (букет: max запаха, max пыльцы; подарок: его запах, fragrance = strong):
S1 астма + (запах|пыльца strong) → BLOCK `ASTHMA_TRIGGER` · S2 астма + medium → WARN `ASTHMA_CAUTION` 0.10 ·
S3 астма неизвестна + strong → WARN `ASTHMA_UNKNOWN` 0.10 · S4 чувствительность strong + запах ≥ medium → BLOCK `SCENT_TOO_STRONG` ·
S5 medium: strong → BLOCK, medium → WARN `SCENT_CAUTION` 0.05 · S6 low + strong → WARN `SCENT_CAUTION` 0.05 ·
S7 неизвестна + strong → WARN `SCENT_UNKNOWN` 0.10.

Подарок: G1 состав nil + известные непустые аллергии → BLOCK `UNKNOWN_COMPOSITION` · G2 состав nil, аллергии неизвестны,
категория ∈ {sweets, fragrance, candle} → WARN 0.15 · G3 пересечение → BLOCK `ALLERGEN_MATCH` · G4 аллергии неизвестны, есть аллергены → WARN `ALLERGY_UNKNOWN` 0.15 ·
G5 PetHazard=true + питомцы есть → BLOCK `PET_HAZARD` · G6 PetHazard=nil + питомцы есть → WARN 0.15 · G7 PetHazard=true + питомцы неизвестны → WARN `PETS_UNKNOWN` 0.15 · G8 запах → S1–S7.

Дубликаты `(code, item, attribute)` схлопываются. Decision: BLOCK > WARN > ALLOW. ConfidenceMultiplier = max(0.2, 1 − Σ штрафов WARN).
`CheckGiftSet`: total > budget → BLOCK `OVER_BUDGET`, повтор → BLOCK `DUPLICATE_ITEM`. `ValidateSubstitution`: замена → `CheckBouquet`, BLOCK → API 409 + audit `substitution.blocked`.
`FilterCandidates` — до ranker. `BestClarifyingQuestion` — атрибут с максимальной суммой штрафов WARN.

## Безопасность и приватность (§13)
- **GiverView**: нарушение из поля с `source = recipient` дарителю показывается только как
  `{"code":"RECIPIENT_RESTRICTION","message":"Не подходит по ограничениям получателя"}`. **Все** ответы роли giver идут через GiverView.
- `GET /recipients/{id}`: поля с `source=recipient` → `{"key":…,"hidden":true}` без значения.
- «Не знаю» нигде (БД, DTO, фронт) не превращается в «нет аллергии»; в форме — отдельная опция.
- Не утверждать медицинскую безопасность: «известных противопоказаний не найдено», не «безопасно»/«гарантированно». Дисклеймер на карточке.
- Геолокация (`lat`/`lon`) не сохраняется: ни БД, ни логи, ни audit. Middleware вырезает их из query (есть тест).
- В логи/метрики не попадают: аллергии, астма, питомцы, дата рождения, геолокация, токены приглашений, промпты с данными получателя.
- JWT, роли giver/florist/manager. Даритель видит только своих получателей (чужой ID → 404). Флорист меняет только свой магазин.
- Приглашения (P1): в БД только SHA-256 токена, одноразовые, со сроком, `noindex`.
- Audit (`audit_events`): импорт каталога, цены/наличие, замены, нумерология, согласия, экспорты, удаление профиля.
- Удаление получателя каскадно удаляет атрибуты, включая дату рождения (FR-N07).
- Секреты только в env; ключ LLM не уходит на фронт; в LLM-промпт не передаются персональные данные получателя.
