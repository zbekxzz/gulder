import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { api, call, errorMessage, type Schemas } from "../../api/client";
import { Button, ErrorState, Field, Input, Select } from "../../components/ui";
import { emotions, occasionTypes, places, relationships } from "../labels";

const schema = z
  .object({
    type: z.string().min(1),
    relationship: z.string(),
    date: z.string(),
    budgetMin: z.coerce.number<string>().min(0, "Не меньше 0"),
    budgetMax: z.coerce.number<string>().positive("Укажите бюджет"),
    place: z.string(),
    formality: z.string(),
    emotion: z.string(),
    numerology: z.boolean(),
  })
  .refine((v) => v.budgetMin <= v.budgetMax, {
    message: "Минимум не больше максимума",
    path: ["budgetMin"],
  });

type Form = z.input<typeof schema>;

/** Повод и бюджет (FR-O01). Нумерология по умолчанию выключена (FR-N01). */
export function OccasionForm({
  recipientId,
  onDone,
}: {
  recipientId: number;
  onDone: (runId: number) => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const { register, handleSubmit, formState } = useForm<Form, unknown, z.output<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      type: "birthday",
      relationship: "",
      date: "",
      budgetMin: "0",
      budgetMax: "20000",
      place: "",
      formality: "",
      emotion: "",
      numerology: false,
    },
  });

  const onSubmit = handleSubmit(async (v) => {
    setError(null);
    try {
      const body: Schemas["OccasionInput"] = {
        recipient_id: recipientId,
        type: v.type as Schemas["OccasionType"],
        budget_min: v.budgetMin,
        budget_max: v.budgetMax,
        numerology_enabled: v.numerology,
        date: v.date || null,
        formality: v.formality ? Number(v.formality) : null,
      };
      if (v.relationship)
        body.relationship = v.relationship as Schemas["OccasionInput"]["relationship"];
      if (v.place) body.place = v.place as Schemas["OccasionInput"]["place"];
      if (v.emotion) body.emotion_goal = v.emotion as Schemas["OccasionInput"]["emotion_goal"];
      const occasion = await call(api.POST("/occasions", { body }));
      const run = await call(api.POST("/recommendations", { body: { occasion_id: occasion.id } }));
      onDone(run.run_id);
    } catch (e) {
      setError(errorMessage(e));
    }
  });

  const opts = (m: Record<string, string>, empty?: string) => (
    <>
      {empty !== undefined && <option value="">{empty}</option>}
      {Object.entries(m).map(([k, l]) => (
        <option key={k} value={k}>
          {l}
        </option>
      ))}
    </>
  );

  return (
    <form className="grid gap-3 sm:grid-cols-2" onSubmit={onSubmit} noValidate>
      <Field label="Повод">
        <Select {...register("type")}>{opts(occasionTypes)}</Select>
      </Field>
      <Field label="Кем приходится">
        <Select {...register("relationship")}>{opts(relationships, "Не указано")}</Select>
      </Field>
      <Field label="Дата">
        <Input type="date" {...register("date")} />
      </Field>
      <Field label="Где вручать">
        <Select {...register("place")}>{opts(places, "Не указано")}</Select>
      </Field>
      <Field label="Бюджет от, ₸" error={formState.errors.budgetMin?.message}>
        <Input inputMode="decimal" {...register("budgetMin")} />
      </Field>
      <Field label="Бюджет до, ₸" error={formState.errors.budgetMax?.message}>
        <Input inputMode="decimal" {...register("budgetMax")} />
      </Field>
      <Field label="Формальность">
        <Select {...register("formality")}>
          <option value="">Не указано</option>
          <option value="1">1 — совсем неформально</option>
          <option value="2">2</option>
          <option value="3">3</option>
          <option value="4">4</option>
          <option value="5">5 — официально</option>
        </Select>
      </Field>
      <Field label="Желаемая эмоция">
        <Select {...register("emotion")}>{opts(emotions, "Не указано")}</Select>
      </Field>
      <label className="flex items-start gap-2 text-sm sm:col-span-2">
        <input type="checkbox" className="mt-0.5" {...register("numerology")} />
        <span>
          Учесть нумерологию{" "}
          <span className="text-stone-500">
            — развлекательная функция, нужна дата рождения в профиле; на проверку ограничений не
            влияет
          </span>
        </span>
      </label>
      {error && (
        <div className="sm:col-span-2">
          <ErrorState error={error} />
        </div>
      )}
      <div className="sm:col-span-2">
        <Button type="submit" disabled={formState.isSubmitting}>
          {formState.isSubmitting ? "Подбираю…" : "Подобрать букеты"}
        </Button>
      </div>
    </form>
  );
}
