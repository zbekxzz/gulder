import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
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
} from "../../components/ui";
import { label, missingLabels, occasionTypes } from "../../features/labels";

type Judgement = Schemas["ExpertJudgement"];

/** Экспертная разметка (RM-07): черновик → проверенная версия; форма оценки 0–3 + причина. */
export function ExpertTab() {
  const qc = useQueryClient();
  const [source, setSource] = useState("draft-1");
  const [target, setTarget] = useState("v1");
  const [index, setIndex] = useState(0);
  const [grade, setGrade] = useState<number | null>(null);
  const [unsafe, setUnsafe] = useState(false);
  const [reason, setReason] = useState("");

  const list = useQuery({
    queryKey: ["judgements", source],
    queryFn: () =>
      call(api.GET("/admin/expert-judgements", { params: { query: { dataset_version: source } } })),
  });
  const done = useQuery({
    queryKey: ["judgements", target],
    queryFn: () =>
      call(api.GET("/admin/expert-judgements", { params: { query: { dataset_version: target } } })),
  });
  const save = useMutation({
    mutationFn: (j: Judgement) =>
      call(
        api.POST("/admin/expert-judgements", {
          body: {
            dataset_version: target,
            case_id: j.case_id,
            recipient_snapshot: j.recipient_snapshot,
            occasion_snapshot: j.occasion_snapshot,
            bouquet_id: j.bouquet_id,
            grade: grade!,
            is_unsafe: unsafe,
            reason,
          },
        }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["judgements", target] });
      setIndex((i) => i + 1);
      setGrade(null);
      setUnsafe(false);
      setReason("");
    },
  });

  const labelled = useMemo(
    () => new Set((done.data?.items ?? []).map((j) => `${j.case_id}/${j.bouquet_id}`)),
    [done.data],
  );
  if (list.isPending) return <Spinner />;
  if (list.error) return <ErrorState error={errorMessage(list.error)} />;
  const items = list.data.items;
  const current = items[Math.min(index, items.length - 1)];

  return (
    <div className="space-y-4">
      <Card>
        <div className="grid gap-3 sm:grid-cols-3">
          <Field label="Исходная версия">
            <Select
              value={source}
              onChange={(e) => {
                setSource(e.target.value);
                setIndex(0);
              }}
            >
              {list.data.versions.map((v) => (
                <option key={v.key} value={v.key}>
                  {v.key} ({v.count})
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Записывать в версию" hint="Черновики (draft-*) не редактируются">
            <Input value={target} onChange={(e) => setTarget(e.target.value)} />
          </Field>
          <div className="text-sm text-stone-600">
            Проверено в {target}: <strong>{done.data?.items.length ?? 0}</strong> из {items.length}
          </div>
        </div>
      </Card>
      {!current ? (
        <EmptyState title="В этой версии нет случаев" />
      ) : (
        <Card>
          <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
            <h2 className="font-semibold">
              Случай {current.case_id}: {current.bouquet_name}
            </h2>
            <div className="flex items-center gap-2 text-sm">
              {labelled.has(`${current.case_id}/${current.bouquet_id}`) && (
                <Badge tone="ok">уже размечено</Badge>
              )}
              <Button variant="ghost" disabled={index === 0} onClick={() => setIndex(index - 1)}>
                ←
              </Button>
              <span>
                {index + 1} / {items.length}
              </span>
              <Button
                variant="ghost"
                disabled={index >= items.length - 1}
                onClick={() => setIndex(index + 1)}
              >
                →
              </Button>
            </div>
          </div>
          <CaseSummary j={current} />
          <p className="mt-2 text-xs text-stone-500">
            Черновая оценка: {current.grade} {current.is_unsafe && "(помечено опасным)"} — «
            {current.reason}»
          </p>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            {[0, 1, 2, 3].map((g) => (
              <Button
                key={g}
                variant={grade === g ? "primary" : "secondary"}
                onClick={() => setGrade(g)}
              >
                {g}
              </Button>
            ))}
            <label className="ml-2 flex items-center gap-1 text-sm">
              <input
                type="checkbox"
                checked={unsafe}
                onChange={(e) => {
                  setUnsafe(e.target.checked);
                  if (e.target.checked) setGrade(0);
                }}
              />
              Опасно для получателя
            </label>
          </div>
          <div className="mt-3">
            <Field label="Причина">
              <Input value={reason} onChange={(e) => setReason(e.target.value)} />
            </Field>
          </div>
          <Button
            className="mt-3"
            disabled={
              grade === null || !reason.trim() || save.isPending || target.startsWith("draft-")
            }
            onClick={() => save.mutate(current)}
          >
            Сохранить оценку
          </Button>
          {save.error && (
            <div className="mt-2">
              <ErrorState error={errorMessage(save.error)} />
            </div>
          )}
        </Card>
      )}
    </div>
  );
}

function CaseSummary({ j }: { j: Judgement }) {
  const profile = j.recipient_snapshot as Record<string, { value: unknown; confidence: string }>;
  const occasion = j.occasion_snapshot as {
    type?: string;
    budget_max?: number;
    emotion_goal?: string;
  };
  const known = Object.entries(profile);
  return (
    <div className="grid gap-2 text-sm sm:grid-cols-2">
      <div>
        <p className="text-xs uppercase text-stone-500">Получатель</p>
        {known.length === 0 && <p className="text-stone-500">Ничего не известно</p>}
        <ul>
          {known.map(([k, a]) => (
            <li key={k}>
              {label(missingLabels, k)}: {JSON.stringify(a.value)}{" "}
              <span className="text-xs text-stone-500">({a.confidence})</span>
            </li>
          ))}
        </ul>
      </div>
      <div>
        <p className="text-xs uppercase text-stone-500">Повод</p>
        <p>
          {label(occasionTypes, occasion.type)}, бюджет до {occasion.budget_max} ₸
        </p>
      </div>
    </div>
  );
}

type Metrics = Schemas["EvalMetrics"];

/** Evaluation dashboard (RM-05): baseline и модель на одном split, NDCG@5 с CI, unsafe, coverage, ECE, robustness. */
export function EvaluationTab() {
  const runs = useQuery({
    queryKey: ["evaluations"],
    queryFn: () => call(api.GET("/research/evaluation")),
  });
  const [selected, setSelected] = useState<number | null>(null);
  const runId = selected ?? runs.data?.items[0]?.id ?? null;
  const detail = useQuery({
    queryKey: ["evaluation", runId],
    enabled: runId !== null,
    queryFn: () =>
      call(api.GET("/research/evaluation/{run_id}", { params: { path: { run_id: runId! } } })),
  });
  const exportMut = useMutation({ mutationFn: () => call(api.POST("/research/export")) });

  if (runs.isPending) return <Spinner />;
  if (runs.error) return <ErrorState error={errorMessage(runs.error)} />;
  if (runs.data.items.length === 0)
    return (
      <EmptyState title="Запусков оценки ещё нет">
        Выполните <code>make eval</code> — результаты появятся здесь.
      </EmptyState>
    );

  const fmt = (v: number | undefined, d = 3) => (v === undefined ? "—" : v.toFixed(d));
  const rows: [string, (m: Metrics) => string][] = [
    [
      "NDCG@5 (95% CI)",
      (m) =>
        `${fmt(m["ndcg@5"]?.mean)} [${fmt(m["ndcg@5"]?.ci95?.[0])}; ${fmt(m["ndcg@5"]?.ci95?.[1])}]`,
    ],
    ["Unsafe rate", (m) => fmt(m.unsafe_rate, 4)],
    ["Unsafe (Rules Engine, каталог)", (m) => fmt(m.unsafe_catalog_rate, 4)],
    ["Нарушения бюджета", (m) => String(m.budget_violations ?? "—")],
    ["Coverage", (m) => fmt(m.coverage)],
    ["ECE", (m) => fmt(m.ece, 4)],
    ["Robustness: падение NDCG@5", (m) => fmt(m.robustness?.ndcg_drop)],
    ["Test случаев / получателей", (m) => `${m.test_cases ?? "—"} / ${m.test_recipients ?? "—"}`],
  ];
  const same = detail.data?.same_split ?? [];

  return (
    <div className="space-y-4">
      <Card>
        <Field label="Запуск">
          <Select value={runId ?? ""} onChange={(e) => setSelected(Number(e.target.value))}>
            {runs.data.items.map((r) => (
              <option key={r.id} value={r.id}>
                #{r.id} · {r.model_version} · {r.dataset_version} · seed {r.seed} ·{" "}
                {new Date(r.created_at).toLocaleString("ru-RU")}
              </option>
            ))}
          </Select>
        </Field>
        {detail.data?.run.dataset_version.startsWith("draft-") && (
          <p className="mt-2 text-sm text-amber-700">
            Черновой набор: метрики проверяют пайплайн, а не качество модели.
          </p>
        )}
      </Card>
      {detail.isPending && <Spinner />}
      {same.length > 0 && (
        <Card className="overflow-x-auto">
          <h2 className="mb-2 font-semibold">Сравнение на split {detail.data?.run.split_id}</h2>
          <table className="w-full text-left text-sm">
            <thead className="text-xs text-stone-500">
              <tr>
                <th className="py-1">Метрика</th>
                {same.map((r) => (
                  <th key={r.id}>{r.model_version}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map(([name, f]) => (
                <tr key={name} className="border-t border-stone-100">
                  <td className="py-1">{name}</td>
                  {same.map((r) => (
                    <td key={r.id} className="font-mono text-xs">
                      {f(r.metrics)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
      <Card>
        <h2 className="mb-2 font-semibold">Исследовательский экспорт</h2>
        <p className="mb-2 text-sm text-stone-600">
          Только пользователи с активным согласием, без идентификаторов и текста, группы меньше 5
          скрыты.
        </p>
        <Button
          variant="secondary"
          onClick={() => exportMut.mutate()}
          disabled={exportMut.isPending}
        >
          Сформировать экспорт
        </Button>
        {exportMut.error && (
          <div className="mt-2">
            <ErrorState error={errorMessage(exportMut.error)} />
          </div>
        )}
        {exportMut.data && (
          <p className="mt-2 text-sm">
            Экспорт #{exportMut.data.export_id}: строк {exportMut.data.rows.length}, скрыто групп{" "}
            {exportMut.data.suppressed_groups}, словарь {exportMut.data.data_dictionary_version}
          </p>
        )}
      </Card>
    </div>
  );
}
