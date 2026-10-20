import { useState } from "react";
import { Badge, Button, Input, Select } from "../../components/ui";
import {
  defaultValue,
  FIELDS,
  isComplete,
  type Confidence,
  type FieldDef,
  type FieldState,
} from "./fields";

const confidenceLabels: Record<Confidence, string> = {
  know: "Знаю",
  assume: "Предполагаю",
  unknown: "Не знаю",
};

export type ProfileValue = Record<string, FieldState>;

type Props = {
  initial: ProfileValue;
  saving?: boolean;
  onSave: (changed: { key: string; value: unknown; confidence: Confidence }[]) => void;
};

/** Редактор профиля: для каждого поля «знаю / предполагаю / не знаю» (§15). */
export function ProfileEditor({ initial, saving, onSave }: Props) {
  const [state, setState] = useState<ProfileValue>(initial);
  const [dirty, setDirty] = useState<Set<string>>(new Set());

  const update = (key: string, patch: Partial<FieldState>) => {
    setState((s) => ({ ...s, [key]: { ...s[key]!, ...patch } }));
    setDirty((d) => new Set(d).add(key));
  };

  const incomplete = FIELDS.filter((f) => dirty.has(f.key) && !isComplete(f, state[f.key]!));

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        onSave(
          [...dirty].map((key) => {
            const s = state[key]!;
            return {
              key,
              confidence: s.confidence,
              value: s.confidence === "unknown" ? null : s.value,
            };
          }),
        );
        setDirty(new Set());
      }}
    >
      {FIELDS.map((f) => (
        <FieldRow key={f.key} def={f} state={state[f.key]!} onChange={(p) => update(f.key, p)} />
      ))}
      {incomplete.length > 0 && (
        <p className="text-sm text-amber-700">
          Укажите значение или выберите «Не знаю»: {incomplete.map((f) => f.label).join(", ")}
        </p>
      )}
      <Button type="submit" disabled={saving || dirty.size === 0 || incomplete.length > 0}>
        {saving ? "Сохраняю…" : "Сохранить профиль"}
      </Button>
    </form>
  );
}

function FieldRow({
  def,
  state,
  onChange,
}: {
  def: FieldDef;
  state: FieldState;
  onChange: (p: Partial<FieldState>) => void;
}) {
  const id = `field-${def.key}`;
  if (state.hidden) {
    return (
      <div className="rounded-lg border border-stone-200 p-3">
        <div className="flex items-center justify-between gap-2">
          <span className="text-sm font-medium">{def.label}</span>
          <Badge tone="info">заполнено получателем, скрыто</Badge>
        </div>
      </div>
    );
  }
  const disabled = state.confidence === "unknown";
  return (
    <fieldset className="rounded-lg border border-stone-200 p-3" aria-labelledby={`${id}-legend`}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span id={`${id}-legend`} className="text-sm font-medium">
          {def.label}
        </span>
        <div
          className="inline-flex overflow-hidden rounded-lg border border-stone-300 text-xs"
          role="radiogroup"
          aria-label={`${def.label}: уверенность`}
        >
          {(Object.keys(confidenceLabels) as Confidence[]).map((c) => (
            <button
              key={c}
              type="button"
              role="radio"
              aria-checked={state.confidence === c}
              className={`px-2.5 py-1 ${state.confidence === c ? "bg-brand-600 text-white" : "bg-white text-stone-700 hover:bg-stone-100"}`}
              onClick={() =>
                onChange({
                  confidence: c,
                  value: c === "unknown" ? null : (state.value ?? defaultValue(def)),
                })
              }
            >
              {confidenceLabels[c]}
            </button>
          ))}
        </div>
      </div>
      {disabled ? (
        <p className="mt-2 text-xs text-stone-500">
          Неизвестно — система будет осторожнее и может задать уточняющий вопрос.
        </p>
      ) : (
        <div className="mt-3">
          <ValueInput def={def} value={state.value} onChange={(value) => onChange({ value })} />
          {state.source === "feedback" && (
            <p className="mt-1 text-xs text-stone-500">Уточнено по вашим отзывам</p>
          )}
        </div>
      )}
    </fieldset>
  );
}

function ValueInput({
  def,
  value,
  onChange,
}: {
  def: FieldDef;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  switch (def.kind) {
    case "multi": {
      const selected = Array.isArray(value) ? (value as string[]) : [];
      return (
        <div>
          <div className="flex flex-wrap gap-1.5">
            {Object.entries(def.options ?? {}).map(([k, label]) => {
              const on = selected.includes(k);
              return (
                <button
                  key={k}
                  type="button"
                  aria-pressed={on}
                  className={`rounded-full border px-2.5 py-1 text-xs ${on ? "border-brand-600 bg-brand-50 text-brand-700" : "border-stone-300 text-stone-700 hover:bg-stone-50"}`}
                  onClick={() => onChange(on ? selected.filter((x) => x !== k) : [...selected, k])}
                >
                  {label}
                </button>
              );
            })}
          </div>
          {selected.length === 0 && def.emptyMeans && (
            <p className="mt-2 text-xs text-stone-600">Ничего не выбрано: «{def.emptyMeans}»</p>
          )}
        </div>
      );
    }
    case "bool":
      return (
        <div className="flex gap-4 text-sm">
          {[
            [true, "Да"],
            [false, "Нет"],
          ].map(([v, label]) => (
            <label key={String(v)} className="flex items-center gap-1.5">
              <input type="radio" checked={value === v} onChange={() => onChange(v)} />
              {label as string}
            </label>
          ))}
        </div>
      );
    case "single":
      return (
        <Select
          value={String(value ?? "")}
          onChange={(e) => onChange(e.target.value)}
          aria-label={def.label}
        >
          {Object.entries(def.options ?? {}).map(([k, label]) => (
            <option key={k} value={k}>
              {label}
            </option>
          ))}
        </Select>
      );
    case "date":
      return (
        <Input
          type="date"
          value={String(value ?? "")}
          max={new Date().toISOString().slice(0, 10)}
          onChange={(e) => onChange(e.target.value)}
          aria-label={def.label}
        />
      );
  }
}
