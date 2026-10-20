# Сверка справочника растений с ASPCA

Дата сверки: 2026-10-20. Источник: ASPCA Animal Poison Control Center, Toxic and Non-Toxic Plant List.
Сверялась токсичность «токсично / нетоксично» для кошек, собак и лошадей — именно она определяет BLOCK в Rules Engine.
Степень (mild/moderate/severe) ASPCA не публикует: она осталась из Приложения B и влияет только на штраф при неизвестных питомцах (F6).

| Таксон | Кошки | Собаки | Лошади | Сверено | Что сказано у ASPCA | Ссылка |
|---|---|---|---|---|---|---|
| Rosa | none | none | none | да | Rosa Species, Rosaceae: Non-Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/rose) |
| Lilium | severe | none | none | да | Lilium species, Liliaceae: Toxic to Cats (kidney failure); Non-Toxic to Dogs, Horses | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/lily) |
| Tulipa | mild | mild | mild | да | Tulipa spp., Liliaceae: Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/tulip) |
| Chrysanthemum | mild | mild | mild | да | Chrysanthemum spp., Compositae: Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/chrysanthemum) |
| Helianthus annuus | none | none | none | да | Страница ASPCA для H. angustifolius (тот же род), Compositae: Non-Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/sunflower) |
| Gerbera jamesonii | none | none | none | да | Gerbera jamesonii, Compositae: Non-Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/aspca-poison-control/toxic-and-non-toxic-plants/gerber-daisy) |
| Dianthus caryophyllus | mild | mild | mild | да | Dianthus caryophyllus, Caryophyllaceae: Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/carnation) |
| Phalaenopsis | none | none | none | да | Phalaenopsis sp., Orchidaceae: Non-Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/aspca-poison-control/toxic-and-non-toxic-plants/phalaenopsis-orchid) |
| Hydrangea macrophylla | moderate | moderate | moderate | да | Страница для H. arborescens, Hydrangeaceae: Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/hydrangea) |
| Paeonia | mild | mild | mild | да | Paeonia officinalis: Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/peony) |
| Narcissus | moderate | moderate | moderate | да | Narcissus spp, Amaryllidaceae: Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/daffodil) |
| Alstroemeria | none | none | none | да | Alstroemeria (ASPCA: Liliaceae): Non-Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/peruvian-lily) |
| Gypsophila paniculata | mild | mild | unknown | **нет** | РАСХОЖДЕНИЕ: страница ASPCA про G. elegans — Non-Toxic (mild GI upset); для G. paniculata вторичные источники дают лёгкую токсичность. Оставлено консервативно mild/mild/unknown, нужна проверка человеком | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/babys-breath) |
| Freesia | none | none | none | да | Отдельной записи в базе ASPCA нет; эксперты ASPCA APCC называют Freesia spp. нетоксичной (статья ASPCA) | [ASPCA](https://www.aspca.org/news/do-you-know-which-flowers-and-plants-are-toxic-pets-our-experts-explain) |
| Eucalyptus | mild | mild | mild | да | Eucalyptus species, Myrtaceae: Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/eucalyptus) |
| Hyacinthus orientalis | mild | mild | mild | да | Hyacinthus orientalis (ASPCA: Liliaceae): Toxic to Dogs, Cats, Horses | [ASPCA](https://www.aspca.org/pet-care/animal-poison-control/toxic-and-non-toxic-plants/hyacinth) |

## Итог

- 15 из 16 строк Приложения B совпали с ASPCA по признаку «токсично / нетоксично»; значения не менялись.
- **Gypsophila paniculata — расхождение.** ASPCA публикует запись для *G. elegans* (нетоксична, возможно лёгкое расстройство ЖКТ),
  а не для *G. paniculata* — гипсофилы из букетов. Pet Poison Helpline и др. описывают *Gypsophila* как лёгкий раздражитель ЖКТ.
  Оставлено консервативно: `mild / mild / unknown`, `verified=false`. Для указанных кошки и собаки это BLOCK, для лошади — тоже BLOCK
  (неизвестная токсичность для указанного питомца, §7.1). Решение нужно принять человеку: ветеринар или ASPCA-консультант.
- Семейства оставлены по современной систематике (Alstroemeriaceae, Asparagaceae), хотя у ASPCA указано Liliaceae.
  Это влияет только на правило F3 (семейство в «рисковых» при неизвестных аллергиях) и аллерген `family:liliaceae`.
- Каталог не содержит родов вне справочника, кроме двух намеренно неопознанных (протея, фисташка), которые блокируются (FR-C03).

## Как повторить

Открыть ссылку из таблицы, сравнить поля «Toxicity / Non-Toxicity». При изменении — поправить `data/plant_taxa.csv`
(значения, `verified`, `aspca_note`) и выполнить `make seed`: справочник обновится в БД, правила начнут действовать сразу.
