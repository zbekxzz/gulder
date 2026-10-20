import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, call, errorMessage, type Schemas } from "../../api/client";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorState,
  Field,
  Input,
  Select,
  Spinner,
  Tabs,
} from "../../components/ui";
import { money } from "../../components/format";
import { giftCategories, label } from "../../features/labels";

type Tab = "bouquets" | "gifts" | "import" | "unconfirmed" | "substitutions";

export function FloristPage() {
  const [tab, setTab] = useState<Tab>("bouquets");
  return (
    <div>
      <h1 className="mb-4 text-2xl font-semibold">Каталог магазина</h1>
      <Tabs
        value={tab}
        onChange={setTab}
        tabs={[
          { id: "bouquets", label: "Букеты" },
          { id: "gifts", label: "Подарки" },
          { id: "import", label: "Импорт CSV" },
          { id: "unconfirmed", label: "Неподтверждённые растения" },
          { id: "substitutions", label: "Замены" },
        ]}
      />
      {tab === "bouquets" && <BouquetsTab />}
      {tab === "gifts" && <GiftsTab />}
      {tab === "import" && <ImportTab />}
      {tab === "unconfirmed" && <UnconfirmedTab />}
      {tab === "substitutions" && <SubstitutionsTab />}
    </div>
  );
}

function OfferCell({ offer }: { offer: Schemas["Offer"] }) {
  const active =
    offer.discount_price != null &&
    offer.discount_until != null &&
    new Date(offer.discount_until) > new Date();
  return (
    <span>
      {money(offer.price)}
      {offer.discount_price != null && (
        <span className={active ? "ml-1 text-brand-700" : "ml-1 text-stone-400 line-through"}>
          {money(offer.discount_price)} до{" "}
          {offer.discount_until ? new Date(offer.discount_until).toLocaleDateString("ru-RU") : "—"}
        </span>
      )}
    </span>
  );
}

/** Редактор наличия, цены и скидки (PUT /availability/{id}). */
function AvailabilityEditor({
  availability,
  offer,
  onDone,
}: {
  availability: Schemas["Availability"];
  offer: Schemas["Offer"];
  onDone: () => void;
}) {
  const qc = useQueryClient();
  const [inStock, setInStock] = useState(availability.in_stock);
  const [price, setPrice] = useState(String(offer.price));
  const [discount, setDiscount] = useState(
    offer.discount_price != null ? String(offer.discount_price) : "",
  );
  const [until, setUntil] = useState(offer.discount_until?.slice(0, 10) ?? "");
  const save = useMutation({
    mutationFn: () =>
      call(
        api.PUT("/availability/{id}", {
          params: { path: { id: availability.id } },
          body: {
            in_stock: inStock,
            price: Number(price),
            ...(discount
              ? {
                  discount_price: Number(discount),
                  discount_until: new Date(`${until}T23:59:59+05:00`).toISOString(),
                }
              : {}),
          },
        }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["florist"] });
      onDone();
    },
  });
  return (
    <div className="grid gap-2 rounded-lg bg-stone-50 p-3 sm:grid-cols-5 sm:items-end">
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={inStock} onChange={(e) => setInStock(e.target.checked)} /> В
        наличии
      </label>
      <Field label="Цена, ₸">
        <Input value={price} onChange={(e) => setPrice(e.target.value)} inputMode="decimal" />
      </Field>
      <Field label="Цена со скидкой">
        <Input value={discount} onChange={(e) => setDiscount(e.target.value)} inputMode="decimal" />
      </Field>
      <Field label="Скидка до">
        <Input
          type="date"
          value={until}
          onChange={(e) => setUntil(e.target.value)}
          disabled={!discount}
        />
      </Field>
      <div className="flex gap-2">
        <Button onClick={() => save.mutate()} disabled={save.isPending || (!!discount && !until)}>
          Сохранить
        </Button>
        <Button variant="ghost" onClick={onDone}>
          Отмена
        </Button>
      </div>
      {save.error && (
        <div className="sm:col-span-5">
          <ErrorState error={errorMessage(save.error)} />
        </div>
      )}
    </div>
  );
}

function BouquetsTab() {
  const q = useQuery({
    queryKey: ["florist", "bouquets"],
    queryFn: () => call(api.GET("/catalog/bouquets")),
  });
  const [editing, setEditing] = useState<number | null>(null);
  if (q.isPending) return <Spinner />;
  if (q.error) return <ErrorState error={errorMessage(q.error)} />;
  if (q.data.items.length === 0)
    return (
      <EmptyState title="Букетов пока нет">Загрузите каталог на вкладке «Импорт CSV».</EmptyState>
    );
  return (
    <div className="space-y-2">
      {q.data.items.map((b) => (
        <Card key={b.id} className="p-3">
          <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
            <div>
              <span className="font-mono text-xs text-stone-500">{b.sku}</span>{" "}
              <span className="font-medium">{b.name}</span>
              <div className="text-xs text-stone-600">
                {b.items.map((i) => (
                  <span key={i.flower_id} className="mr-2">
                    {i.trade_name} × {i.stems}
                    {!i.taxon_confirmed && <Badge tone="danger">не подтверждён</Badge>}
                  </span>
                ))}
              </div>
            </div>
            <div className="flex items-center gap-3">
              <OfferCell offer={b.offer} />
              <Badge tone={b.availability.in_stock ? "ok" : "warn"}>
                {b.availability.in_stock ? "в наличии" : "нет"}
              </Badge>
              <Button
                variant="secondary"
                onClick={() => setEditing(editing === b.id ? null : b.id)}
              >
                Цена и наличие
              </Button>
            </div>
          </div>
          {editing === b.id && (
            <div className="mt-2">
              <AvailabilityEditor
                availability={b.availability}
                offer={b.offer}
                onDone={() => setEditing(null)}
              />
            </div>
          )}
        </Card>
      ))}
    </div>
  );
}

function GiftsTab() {
  const q = useQuery({
    queryKey: ["florist", "gifts"],
    queryFn: () => call(api.GET("/catalog/gifts")),
  });
  const [editing, setEditing] = useState<number | null>(null);
  if (q.isPending) return <Spinner />;
  if (q.error) return <ErrorState error={errorMessage(q.error)} />;
  if (q.data.items.length === 0) return <EmptyState title="Подарков пока нет" />;
  return (
    <div className="space-y-2">
      {q.data.items.map((g) => (
        <Card key={g.id} className="p-3">
          <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
            <div>
              <span className="font-mono text-xs text-stone-500">{g.sku}</span>{" "}
              <span className="font-medium">{g.name}</span>{" "}
              <Badge>{label(giftCategories, g.category)}</Badge>
              <div className="text-xs text-stone-600">
                {g.allergen_codes === null ? (
                  <Badge tone="warn">состав неизвестен</Badge>
                ) : g.allergen_codes.length === 0 ? (
                  "аллергенов нет (проверено)"
                ) : (
                  `аллергены: ${g.allergen_codes.join(", ")}`
                )}
              </div>
            </div>
            <div className="flex items-center gap-3">
              <OfferCell offer={g.offer} />
              <Badge tone={g.availability.in_stock ? "ok" : "warn"}>
                {g.availability.in_stock ? "в наличии" : "нет"}
              </Badge>
              <Button
                variant="secondary"
                onClick={() => setEditing(editing === g.id ? null : g.id)}
              >
                Цена и наличие
              </Button>
            </div>
          </div>
          {editing === g.id && (
            <div className="mt-2">
              <AvailabilityEditor
                availability={g.availability}
                offer={g.offer}
                onDone={() => setEditing(null)}
              />
            </div>
          )}
        </Card>
      ))}
    </div>
  );
}

function ImportTab() {
  const qc = useQueryClient();
  const [kind, setKind] = useState<"flowers" | "bouquets" | "gifts">("bouquets");
  const [file, setFile] = useState<File | null>(null);
  const upload = useMutation({
    mutationFn: () =>
      call(
        api.POST("/catalog/import", {
          params: { query: { kind } },
          body: { file: file as unknown as string },
          bodySerializer: (b) => {
            const fd = new FormData();
            fd.append("file", b.file as unknown as Blob);
            return fd;
          },
        }),
      ),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["florist"] }),
  });
  return (
    <Card>
      <p className="mb-3 text-sm text-stone-600">
        Формат файлов описан в документации (docs/api.md). Строки с ошибками пропускаются, остальные
        загружаются. Неизвестный род растения создаётся неподтверждённым — такие букеты не
        рекомендуются, пока вы не выберете таксон.
      </p>
      <form
        className="flex flex-col gap-2 sm:flex-row sm:items-end"
        onSubmit={(e) => {
          e.preventDefault();
          if (file) upload.mutate();
        }}
      >
        <Field label="Что загружаем">
          <Select value={kind} onChange={(e) => setKind(e.target.value as typeof kind)}>
            <option value="flowers">Цветы</option>
            <option value="bouquets">Букеты</option>
            <option value="gifts">Подарки</option>
          </Select>
        </Field>
        <Field label="CSV-файл">
          <Input
            type="file"
            accept=".csv,text/csv"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          />
        </Field>
        <Button type="submit" disabled={!file || upload.isPending}>
          Загрузить
        </Button>
      </form>
      {upload.error && (
        <div className="mt-3">
          <ErrorState error={errorMessage(upload.error)} />
        </div>
      )}
      {upload.data && (
        <div className="mt-4 text-sm">
          <p>
            Добавлено: <strong>{upload.data.imported}</strong>, обновлено:{" "}
            <strong>{upload.data.updated}</strong>, ошибок:{" "}
            <strong>{upload.data.errors.length}</strong>
          </p>
          {upload.data.errors.length > 0 && (
            <table className="mt-2 w-full text-left text-xs">
              <thead>
                <tr className="text-stone-500">
                  <th className="py-1">Строка</th>
                  <th>Поле</th>
                  <th>Ошибка</th>
                </tr>
              </thead>
              <tbody>
                {upload.data.errors.map((er, i) => (
                  <tr key={i} className="border-t border-stone-100">
                    <td className="py-1">{er.line}</td>
                    <td>{er.field || "—"}</td>
                    <td>{er.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </Card>
  );
}

function UnconfirmedTab() {
  const qc = useQueryClient();
  const flowers = useQuery({
    queryKey: ["florist", "unconfirmed"],
    queryFn: () => call(api.GET("/catalog/flowers", { params: { query: { unconfirmed: true } } })),
  });
  const taxa = useQuery({ queryKey: ["taxa"], queryFn: () => call(api.GET("/catalog/taxa")) });
  const [choice, setChoice] = useState<Record<number, number>>({});
  const confirm = useMutation({
    mutationFn: (v: { flower: number; taxon: number }) =>
      call(
        api.PUT("/catalog/flowers/{id}/taxon", {
          params: { path: { id: v.flower } },
          body: { taxon_id: v.taxon },
        }),
      ),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["florist"] }),
  });
  if (flowers.isPending || taxa.isPending) return <Spinner />;
  if (flowers.error) return <ErrorState error={errorMessage(flowers.error)} />;
  if (flowers.data.items.length === 0) return <EmptyState title="Все растения подтверждены" />;
  return (
    <Card>
      <p className="mb-3 text-sm text-stone-600">
        Пока растение не подтверждено, букеты с ним не рекомендуются никому (FR-C03). Выберите вид
        из справочника.
      </p>
      <ul className="space-y-2">
        {flowers.data.items.map((f) => (
          <li key={f.id} className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <span className="flex-1 text-sm font-medium">{f.trade_name}</span>
            <Select
              aria-label={`Таксон для ${f.trade_name}`}
              value={choice[f.id] ?? ""}
              onChange={(e) => setChoice({ ...choice, [f.id]: Number(e.target.value) })}
            >
              <option value="">Выберите вид…</option>
              {taxa.data?.items.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.scientific_name} ({t.family})
                </option>
              ))}
            </Select>
            <Button
              disabled={!choice[f.id] || confirm.isPending}
              onClick={() => confirm.mutate({ flower: f.id, taxon: choice[f.id]! })}
            >
              Подтвердить
            </Button>
          </li>
        ))}
      </ul>
      {confirm.error && (
        <div className="mt-2">
          <ErrorState error={errorMessage(confirm.error)} />
        </div>
      )}
    </Card>
  );
}

type Validation = { decision: string; reasons: Schemas["GiverReason"][]; blocked: boolean };

/** Замены (FR-S01, AC-3): выбор цветка → мгновенная проверка; опасная — кнопка неактивна, API всё равно проверяет. */
export function SubstitutionsTab() {
  const bouquets = useQuery({
    queryKey: ["florist", "bouquets"],
    queryFn: () => call(api.GET("/catalog/bouquets")),
  });
  const flowers = useQuery({
    queryKey: ["florist", "flowers"],
    queryFn: () => call(api.GET("/catalog/flowers")),
  });
  const [bouquetId, setBouquetId] = useState<number | null>(null);
  const [fromId, setFromId] = useState<number | null>(null);
  const [toId, setToId] = useState<number | null>(null);
  const [occasionId, setOccasionId] = useState("");
  const [appliedKey, setAppliedKey] = useState<string | null>(null);

  const bouquet = bouquets.data?.items.find((b) => b.id === bouquetId);
  const ready = !!bouquetId && !!fromId && !!toId;
  const key = [bouquetId, fromId, toId, occasionId].join(":");

  const request = (dryRun: boolean) => {
    const body: {
      from_flower_id: number;
      to_flower_id: number;
      dry_run: boolean;
      occasion_id?: number;
    } = {
      from_flower_id: fromId!,
      to_flower_id: toId!,
      dry_run: dryRun,
    };
    if (occasionId) body.occasion_id = Number(occasionId);
    return api.POST("/bouquets/{id}/substitutions/validate", {
      params: { path: { id: bouquetId! } },
      body,
    });
  };

  // мгновенная проверка без записи в журнал (dry_run); 409 — это ответ «нельзя», а не ошибка
  const validation = useQuery({
    queryKey: ["substitution", key],
    enabled: ready,
    queryFn: async (): Promise<Validation> => {
      const { data, error, response } = await request(true);
      if (response.ok && data)
        return { decision: data.decision, reasons: data.giver_view.reasons, blocked: false };
      const body = error as {
        error?: { message?: string; details?: { giver_view?: Schemas["GiverView"] } };
      };
      if (response.status === 409)
        return {
          decision: "block",
          reasons: body.error?.details?.giver_view?.reasons ?? [],
          blocked: true,
        };
      return {
        decision: "error",
        reasons: [{ code: "error", message: body.error?.message ?? "Ошибка проверки" }],
        blocked: true,
      };
    },
  });
  const check = ready ? validation.data : undefined;
  const checking = ready && validation.isFetching;

  const apply = useMutation({
    mutationFn: () => call(request(false)),
    onSuccess: () => setAppliedKey(key),
  });

  if (bouquets.isPending || flowers.isPending) return <Spinner />;
  return (
    <Card>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Букет">
          <Select
            aria-label="Букет"
            value={bouquetId ?? ""}
            onChange={(e) => {
              setBouquetId(Number(e.target.value) || null);
              setFromId(null);
            }}
          >
            <option value="">Выберите букет…</option>
            {bouquets.data?.items.map((b) => (
              <option key={b.id} value={b.id}>
                {b.sku} — {b.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field
          label="Заказ (повод), необязательно"
          hint="Если указан — проверка учитывает ограничения получателя (без раскрытия деталей)"
        >
          <Input
            aria-label="Повод"
            inputMode="numeric"
            value={occasionId}
            onChange={(e) => setOccasionId(e.target.value.replace(/\D/g, ""))}
          />
        </Field>
        <Field label="Заменить цветок">
          <Select
            aria-label="Заменить цветок"
            value={fromId ?? ""}
            disabled={!bouquet}
            onChange={(e) => setFromId(Number(e.target.value) || null)}
          >
            <option value="">Выберите…</option>
            {bouquet?.items.map((i) => (
              <option key={i.flower_id} value={i.flower_id}>
                {i.trade_name} × {i.stems}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="На цветок">
          <Select
            aria-label="На цветок"
            value={toId ?? ""}
            onChange={(e) => setToId(Number(e.target.value) || null)}
          >
            <option value="">Выберите…</option>
            {flowers.data?.items.map((f) => (
              <option key={f.id} value={f.id}>
                {f.trade_name}
              </option>
            ))}
          </Select>
        </Field>
      </div>

      {checking && <Spinner label="Проверяю…" />}
      {check && !checking && (
        <div
          className={`mt-3 rounded-lg p-3 text-sm ${check.blocked ? "bg-red-50 text-red-800" : "bg-emerald-50 text-emerald-800"}`}
          data-testid="substitution-result"
        >
          <p className="font-medium">
            {check.blocked
              ? "Замена запрещена"
              : check.decision === "warn"
                ? "Можно, но есть предупреждения"
                : "Известных противопоказаний не найдено"}
          </p>
          <ul className="mt-1 list-disc pl-5">
            {check.reasons.map((r) => (
              <li key={r.code + r.message}>{r.message}</li>
            ))}
          </ul>
        </div>
      )}
      <div className="mt-3 flex items-center gap-3">
        <Button
          disabled={!check || check.blocked || checking || apply.isPending}
          onClick={() => apply.mutate()}
        >
          Применить замену
        </Button>
        {appliedKey === key && (
          <span className="text-sm text-emerald-700">Замена согласована и записана в журнал</span>
        )}
      </div>
      {apply.error && (
        <div className="mt-2">
          <ErrorState error={errorMessage(apply.error)} />
        </div>
      )}
    </Card>
  );
}
