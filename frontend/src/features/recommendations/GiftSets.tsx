import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api, call, errorMessage, type Schemas } from "../../api/client";
import { Badge, Button, Card, ErrorState, Field, Input } from "../../components/ui";
import { money } from "../../components/format";
import { giftCategories, label } from "../labels";

type Set = Schemas["GiftSet"];

/** Наборы «букет + подарки» от ассистента (FR-G01–G04): итог, остаток, «убрать» у каждой позиции. */
export function GiftSets({ runId, defaultBudget }: { runId: number; defaultBudget: number }) {
  const [budget, setBudget] = useState(String(defaultBudget));
  const [sets, setSets] = useState<Set[]>([]);
  const [generator, setGenerator] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const build = useMutation({
    mutationFn: () =>
      call(api.POST("/gift-sets", { body: { run_id: runId, budget: Number(budget) } })),
    onSuccess: (r) => {
      setSets(r.sets);
      setGenerator(r.generator);
      setMessage(r.message ?? null);
    },
  });
  const exclude = useMutation({
    mutationFn: (v: { set: Set; type: "bouquet" | "gift"; id: number }) =>
      call(
        api.POST("/gift-sets/{id}/exclude", {
          params: { path: { id: v.set.id } },
          body: { item_type: v.type, item_id: v.id },
        }),
      ),
    onSuccess: (r, v) => {
      setSets((all) => all.map((s) => (s.id === v.set.id ? r.set : s)));
      setMessage(r.message ?? null);
    },
  });

  return (
    <Card>
      <h2 className="mb-3 font-semibold">Набор «букет + подарки»</h2>
      <form
        className="flex flex-col gap-2 sm:flex-row sm:items-end"
        onSubmit={(e) => {
          e.preventDefault();
          build.mutate();
        }}
      >
        <Field label="Общий бюджет, ₸">
          <Input inputMode="decimal" value={budget} onChange={(e) => setBudget(e.target.value)} />
        </Field>
        <Button type="submit" disabled={build.isPending || !(Number(budget) > 0)}>
          {build.isPending ? "Собираю…" : "Собрать наборы"}
        </Button>
      </form>
      {build.error && (
        <div className="mt-2">
          <ErrorState error={errorMessage(build.error)} />
        </div>
      )}
      {exclude.error && (
        <div className="mt-2">
          <ErrorState error={errorMessage(exclude.error)} />
        </div>
      )}
      {message && <p className="mt-2 text-sm text-amber-700">{message}</p>}
      {generator && (
        <p className="mt-2 text-xs text-stone-500">
          {generator === "llm"
            ? "Наборы предложил AI-ассистент и проверила система"
            : "Наборы собраны автоматически по правилам"}
        </p>
      )}
      <div className="mt-4 grid gap-3 md:grid-cols-3">
        {sets.map((s) => (
          <div
            key={s.id}
            className="rounded-lg border border-stone-200 p-3 text-sm"
            data-testid="gift-set"
          >
            <div className="mb-2 flex items-center justify-between">
              <span className="font-semibold">{s.title || "Набор"}</span>
              <Badge tone={s.generator === "llm" ? "info" : "neutral"}>
                {s.generator === "llm" ? "AI" : "авто"}
              </Badge>
            </div>
            <ul className="space-y-1.5">
              <Item
                name={`💐 ${s.bouquet.name}`}
                price={s.bouquet.discount_price ?? s.bouquet.price}
                onRemove={() =>
                  exclude.mutate({ set: s, type: "bouquet", id: s.bouquet.bouquet_id })
                }
                disabled={exclude.isPending}
              />
              {s.gifts.map((g) => (
                <Item
                  key={g.id}
                  name={`${g.name} · ${label(giftCategories, g.category)}`}
                  price={g.discount_price ?? g.price}
                  onRemove={() => exclude.mutate({ set: s, type: "gift", id: g.id })}
                  disabled={exclude.isPending}
                />
              ))}
            </ul>
            <div className="mt-2 border-t border-stone-100 pt-2">
              <div className="flex justify-between font-medium">
                <span>Итого</span>
                <span data-testid="set-total">{money(s.total_price)}</span>
              </div>
              <div className="flex justify-between text-xs text-stone-500">
                <span>Остаток бюджета</span>
                <span>{money(s.budget_left)}</span>
              </div>
            </div>
            {s.reasons.length > 0 && (
              <ul className="mt-2 list-disc pl-4 text-xs text-stone-600">
                {s.reasons.slice(0, 4).map((r) => (
                  <li key={r}>{r}</li>
                ))}
              </ul>
            )}
            {s.warnings.length > 0 && (
              <ul className="mt-2 text-xs text-red-800">
                {s.warnings.map((w) => (
                  <li key={w.code + w.message}>⚠ {w.message}</li>
                ))}
              </ul>
            )}
          </div>
        ))}
      </div>
    </Card>
  );
}

function Item({
  name,
  price,
  onRemove,
  disabled,
}: {
  name: string;
  price: number;
  onRemove: () => void;
  disabled: boolean;
}) {
  return (
    <li className="flex items-center justify-between gap-2">
      <span>{name}</span>
      <span className="flex items-center gap-1 whitespace-nowrap">
        {money(price)}
        <button
          className="rounded px-1.5 text-xs text-stone-500 hover:bg-stone-100"
          onClick={onRemove}
          disabled={disabled}
          aria-label={`Убрать ${name}`}
        >
          убрать
        </button>
      </span>
    </li>
  );
}
