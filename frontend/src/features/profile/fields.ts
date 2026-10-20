import * as L from "../labels";

export type FieldKind = "multi" | "bool" | "single" | "date";

export type FieldDef = {
  key: string;
  label: string;
  kind: FieldKind;
  options?: Record<string, string>;
  /** Подсказка для пустого списка при «знаю»: явное «нет». */
  emptyMeans?: string;
};

/** Поля профиля (§6.1, FR-P02). Порядок — как в форме. */
export const FIELDS: FieldDef[] = [
  {
    key: "allergies",
    label: "Аллергии",
    kind: "multi",
    options: L.allergens,
    emptyMeans: "Аллергий нет",
  },
  { key: "asthma", label: "Астма", kind: "bool" },
  {
    key: "pets",
    label: "Домашние животные",
    kind: "multi",
    options: L.pets,
    emptyMeans: "Животных нет",
  },
  {
    key: "scent_sensitivity",
    label: "Чувствительность к запахам",
    kind: "single",
    options: L.levels,
  },
  {
    key: "liked_flowers",
    label: "Любимые цветы",
    kind: "multi",
    options: L.genera,
    emptyMeans: "Особых предпочтений нет",
  },
  {
    key: "disliked_flowers",
    label: "Нелюбимые цветы",
    kind: "multi",
    options: L.genera,
    emptyMeans: "Таких нет",
  },
  {
    key: "liked_colors",
    label: "Любимые цвета",
    kind: "multi",
    options: L.colors,
    emptyMeans: "Особых предпочтений нет",
  },
  {
    key: "disliked_colors",
    label: "Нелюбимые цвета",
    kind: "multi",
    options: L.colors,
    emptyMeans: "Таких нет",
  },
  { key: "style", label: "Стиль", kind: "single", options: L.styles },
  { key: "composition_format", label: "Формат композиции", kind: "single", options: L.formats },
  { key: "birth_date", label: "Дата рождения (только для нумерологии)", kind: "date" },
];

export type Confidence = "know" | "assume" | "unknown";

/** Состояние поля в форме. «Не знаю» — отдельная опция, а не пустое значение (§13, AC-2). */
export type FieldState = {
  confidence: Confidence;
  value: unknown;
  hidden?: boolean;
  source?: string;
};

export function defaultValue(f: FieldDef): unknown {
  switch (f.kind) {
    case "multi":
      return [];
    case "bool":
      return false;
    case "single":
      return Object.keys(f.options ?? {})[0] ?? "";
    case "date":
      return "";
  }
}

/** Значение готово к отправке: при «знаю/предполагаю» оно должно быть задано. */
export function isComplete(f: FieldDef, s: FieldState): boolean {
  if (s.confidence === "unknown" || s.hidden) return true;
  if (f.kind === "date") return typeof s.value === "string" && s.value !== "";
  return s.value !== undefined && s.value !== null && s.value !== "";
}
