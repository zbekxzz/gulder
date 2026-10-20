import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, call, errorMessage, type Schemas } from "../../api/client";
import { Badge, Button, Card, ErrorState, Input, Spinner, Tabs } from "../../components/ui";

type Tab = "rules" | "numerology" | "quality" | "analytics";

export function ManagerPage({
  extraTabs,
}: {
  extraTabs?: { id: string; label: string; render: () => React.ReactNode }[];
}) {
  const [tab, setTab] = useState<string>("rules");
  const base: { id: Tab; label: string }[] = [
    { id: "rules", label: "Правила и справочник" },
    { id: "numerology", label: "Нумерология" },
    { id: "quality", label: "Качество данных" },
    { id: "analytics", label: "Аналитика" },
  ];
  const extra = extraTabs ?? [];
  return (
    <div>
      <h1 className="mb-4 text-2xl font-semibold">Управление</h1>
      <Tabs
        value={tab}
        onChange={setTab}
        tabs={[...base, ...extra.map(({ id, label }) => ({ id, label }))]}
      />
      {tab === "rules" && <RulesTab />}
      {tab === "numerology" && <NumerologyTab />}
      {tab === "quality" && <QualityTab />}
      {tab === "analytics" && <AnalyticsTab />}
      {extra.find((t) => t.id === tab)?.render()}
    </div>
  );
}

function RulesTab() {
  const info = useQuery({
    queryKey: ["admin", "rules"],
    queryFn: () => call(api.GET("/admin/rules")),
  });
  const taxa = useQuery({ queryKey: ["taxa"], queryFn: () => call(api.GET("/catalog/taxa")) });
  if (info.isPending || taxa.isPending) return <Spinner />;
  if (info.error) return <ErrorState error={errorMessage(info.error)} />;
  return (
    <div className="space-y-4">
      <Card>
        <p className="text-sm">
          Rules Engine: <strong>{info.data.rules_version}</strong> · модель ранжирования:{" "}
          <strong>{info.data.model_version}</strong> · вес нумерологии:{" "}
          {info.data.numerology_weight} ({info.data.numerology_label})
        </p>
        <div className="mt-2 flex flex-wrap gap-2 text-xs">
          {Object.entries(info.data.weights).map(([k, v]) => (
            <Badge key={k}>
              {k}: {v}
            </Badge>
          ))}
        </div>
      </Card>
      <Card className="overflow-x-auto">
        <h2 className="mb-2 font-semibold">Справочник растений</h2>
        <table className="w-full text-left text-xs">
          <thead className="text-stone-500">
            <tr>
              <th className="py-1">Вид</th>
              <th>Семейство</th>
              <th>Кошки</th>
              <th>Собаки</th>
              <th>Лошади</th>
              <th>Пыльца</th>
              <th>ASPCA</th>
              <th>Сверено</th>
            </tr>
          </thead>
          <tbody>
            {taxa.data?.items.map((t) => (
              <tr key={t.id} className="border-t border-stone-100">
                <td className="py-1 font-medium">{t.scientific_name}</td>
                <td>{t.family}</td>
                {(["cat", "dog", "horse"] as const).map((sp) => (
                  <td key={sp}>
                    <Tox v={t.pet_toxicity[sp]} />
                  </td>
                ))}
                <td>{t.pollen}</td>
                <td>
                  {t.aspca_url ? (
                    <a
                      className="text-brand-700 underline"
                      href={t.aspca_url}
                      target="_blank"
                      rel="noreferrer"
                    >
                      ссылка
                    </a>
                  ) : (
                    "—"
                  )}
                </td>
                <td>
                  {t.verified ? <Badge tone="ok">да</Badge> : <Badge tone="warn">черновик</Badge>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </div>
  );
}

function Tox({ v }: { v?: Schemas["Toxicity"] }) {
  const tone =
    v === "none" ? "ok" : v === "mild" ? "warn" : v === "unknown" || !v ? "neutral" : "danger";
  return <Badge tone={tone}>{v ?? "unknown"}</Badge>;
}

const NUMBERS = [1, 2, 3, 4, 5, 6, 7, 8, 9, 11, 22, 33];

function NumerologyTab() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["admin", "numerology"],
    queryFn: () => call(api.GET("/admin/numerology")),
  });
  const [rows, setRows] = useState<Record<
    number,
    { flowers: string; colors: string; meaning: string }
  > | null>(null);
  const save = useMutation({
    mutationFn: (body: Schemas["NumerologyRow"][]) =>
      call(api.PUT("/admin/numerology", { body: { rows: body } })),
    onSuccess: () => {
      setRows(null);
      void qc.invalidateQueries({ queryKey: ["admin", "numerology"] });
    },
  });
  if (q.isPending) return <Spinner />;
  if (q.error) return <ErrorState error={errorMessage(q.error)} />;
  const active = q.data.active;
  const current =
    rows ??
    Object.fromEntries(
      NUMBERS.map((n) => {
        const r = active?.rows?.find((x) => x.number === n);
        return [
          n,
          {
            flowers: r?.flowers.join(", ") ?? "",
            colors: r?.colors.join(", ") ?? "",
            meaning: r?.meaning ?? "",
          },
        ];
      }),
    );
  const set = (n: number, k: "flowers" | "colors" | "meaning", v: string) =>
    setRows({ ...current, [n]: { ...current[n]!, [k]: v } });
  const split = (s: string) =>
    s
      .split(",")
      .map((x) => x.trim())
      .filter(Boolean);

  return (
    <div className="space-y-4">
      <Card className="overflow-x-auto">
        <p className="mb-2 text-sm text-stone-600">
          Активная версия: <strong>{active?.version ?? "—"}</strong>. Сохранение создаёт новую
          версию. Нумерология — {q.data.label}, вес в score не больше 15% и на проверку ограничений
          не влияет.
        </p>
        <table className="w-full text-left text-xs">
          <thead className="text-stone-500">
            <tr>
              <th className="py-1">Число</th>
              <th>Роды (латиницей, через запятую)</th>
              <th>Цвета</th>
              <th>Значение</th>
            </tr>
          </thead>
          <tbody>
            {NUMBERS.map((n) => (
              <tr key={n}>
                <td className="py-1 font-medium">{n}</td>
                <td>
                  <Input
                    value={current[n]!.flowers}
                    onChange={(e) => set(n, "flowers", e.target.value)}
                    aria-label={`Роды для ${n}`}
                  />
                </td>
                <td>
                  <Input
                    value={current[n]!.colors}
                    onChange={(e) => set(n, "colors", e.target.value)}
                    aria-label={`Цвета для ${n}`}
                  />
                </td>
                <td>
                  <Input
                    value={current[n]!.meaning}
                    onChange={(e) => set(n, "meaning", e.target.value)}
                    aria-label={`Значение для ${n}`}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <div className="mt-3 flex gap-2">
          <Button
            disabled={!rows || save.isPending}
            onClick={() =>
              save.mutate(
                NUMBERS.filter((n) => current[n]!.meaning.trim()).map((n) => ({
                  number: n as Schemas["NumerologyRow"]["number"],
                  flowers: split(current[n]!.flowers),
                  colors: split(current[n]!.colors),
                  meaning: current[n]!.meaning,
                })),
              )
            }
          >
            Сохранить новую версию
          </Button>
          {rows && (
            <Button variant="ghost" onClick={() => setRows(null)}>
              Сбросить
            </Button>
          )}
        </div>
        {save.error && (
          <div className="mt-2">
            <ErrorState error={errorMessage(save.error)} />
          </div>
        )}
      </Card>
      <Card>
        <h2 className="mb-2 font-semibold">История версий</h2>
        <ul className="text-sm">
          {q.data.versions.map((v) => (
            <li key={v.id}>
              Версия {v.version} — {new Date(v.created_at).toLocaleString("ru-RU")}{" "}
              {v.is_active && <Badge tone="ok">активна</Badge>}
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}

const qualityTitles: Record<string, string> = {
  taxa_without_aspca: "Растения без ссылки ASPCA",
  taxa_unverified: "Токсичность не сверена с ASPCA",
  taxa_image_without_license: "Изображения без лицензии",
  gifts_without_composition: "Подарки с неизвестным составом",
  bouquets_without_image: "Букеты без фото",
  unconfirmed_flowers: "Неподтверждённые растения",
};

function QualityTab() {
  const q = useQuery({
    queryKey: ["admin", "quality"],
    queryFn: () => call(api.GET("/admin/data-quality")),
  });
  if (q.isPending) return <Spinner />;
  if (q.error) return <ErrorState error={errorMessage(q.error)} />;
  return (
    <div className="grid gap-3 md:grid-cols-2">
      {Object.entries(qualityTitles).map(([key, title]) => {
        const items = q.data[key] ?? [];
        return (
          <Card key={key}>
            <div className="mb-2 flex items-center justify-between">
              <h2 className="font-semibold">{title}</h2>
              <Badge tone={items.length ? "warn" : "ok"}>{items.length}</Badge>
            </div>
            <ul className="max-h-48 overflow-y-auto text-xs text-stone-700">
              {items.map((i) => (
                <li key={i.id}>
                  {i.name}
                  {i.shop ? ` — ${i.shop}` : ""}
                </li>
              ))}
            </ul>
          </Card>
        );
      })}
    </div>
  );
}

function AnalyticsTab() {
  const q = useQuery({
    queryKey: ["admin", "analytics"],
    queryFn: () => call(api.GET("/admin/analytics")),
  });
  if (q.isPending) return <Spinner />;
  if (q.error) return <ErrorState error={errorMessage(q.error)} />;
  const a = q.data;
  const tiles: [string, string | number][] = [
    ["Запусков рекомендаций", a.runs_total],
    ["С нумерологией", a.numerology_runs],
    ["Отзывов", a.feedback_total],
    ["Средняя оценка", a.avg_rating?.toFixed(2) ?? "—"],
    ["Наборов ассистента", a.gift_sets_total],
    ["Из них от LLM", a.gift_sets_llm],
    ["Заблокированных замен", a.substitutions_blocked],
  ];
  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-4">
        {tiles.map(([l, v]) => (
          <Card key={l}>
            <p className="text-xs text-stone-500">{l}</p>
            <p className="text-2xl font-semibold">{v}</p>
          </Card>
        ))}
      </div>
      <Card>
        <h2 className="mb-2 font-semibold">Причины исключений Rules Engine</h2>
        <ul className="text-sm">
          {a.exclusion_codes.map((c) => (
            <li key={c.key} className="flex justify-between border-t border-stone-100 py-1">
              <span className="font-mono text-xs">{c.key}</span>
              <span>{c.count}</span>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}
