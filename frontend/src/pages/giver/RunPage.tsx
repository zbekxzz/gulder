import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { lazy, Suspense, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, call, errorMessage, type Schemas } from "../../api/client";
import { Badge, Button, Card, EmptyState, ErrorState, Spinner } from "../../components/ui";
import type { Point } from "../../features/map/ShopsMap";
import { BouquetCard } from "../../features/recommendations/BouquetCard";
import { GiftSets } from "../../features/recommendations/GiftSets";
import { quickAnswers } from "../../features/recommendations/questions";

const ShopsMap = lazy(() =>
  import("../../features/map/ShopsMap").then((m) => ({ default: m.ShopsMap })),
);

type Filter = "nearby" | "cheaper" | "discount";
const filterLabels: Record<Filter, string> = {
  nearby: "Рядом",
  cheaper: "Дешевле",
  discount: "Со скидкой",
};

/** Top-5, уточняющий вопрос, фильтры, карта, наборы и отзывы (§15, экран дарителя). */
export function RunPage() {
  const runId = Number(useParams().id);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [filters, setFilters] = useState<Filter[]>([]);
  const [point, setPoint] = useState<Point | null>(null);
  const [geoNote, setGeoNote] = useState<string | null>(null);
  const [cheaperThan, setCheaperThan] = useState<number | null>(null);
  const [feedbackSent, setFeedbackSent] = useState<Record<number, string>>({});

  const run = useQuery({
    queryKey: ["run", runId],
    queryFn: () => call(api.GET("/recommendations/{id}", { params: { path: { id: runId } } })),
  });
  const occasion = useQuery({
    queryKey: ["occasion", run.data?.occasion_id],
    enabled: !!run.data,
    queryFn: () =>
      call(api.GET("/occasions/{id}", { params: { path: { id: run.data!.occasion_id } } })),
  });
  const shops = useQuery({ queryKey: ["shops"], queryFn: () => call(api.GET("/shops")) });

  const offersEnabled =
    filters.length > 0 &&
    (!filters.includes("nearby") || !!point) &&
    (!filters.includes("cheaper") || !!cheaperThan);
  const offers = useQuery({
    queryKey: ["offers", runId, filters.join(","), point?.lat, point?.lon, cheaperThan],
    enabled: offersEnabled,
    queryFn: () =>
      call(
        api.GET("/offers", {
          params: {
            query: {
              run_id: runId,
              filter: filters.join(","),
              ...(point ? { lat: point.lat, lon: point.lon } : {}),
              ...(cheaperThan ? { bouquet_id: cheaperThan } : {}),
            },
          },
        }),
      ),
  });

  // Ответ на уточняющий вопрос: обновить профиль и пересчитать (FR-R04)
  const answer = useMutation({
    mutationFn: async (v: { attribute: string; value: unknown }) => {
      await call(
        api.PUT("/recipients/{id}", {
          params: { path: { id: run.data!.recipient_id } },
          body: {
            attributes: [
              {
                key: v.attribute as Schemas["AttributeKey"],
                value: v.value,
                confidence: "know",
                source: "giver",
              },
            ],
          },
        }),
      );
      return call(api.POST("/recommendations", { body: { occasion_id: run.data!.occasion_id } }));
    },
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: ["recipient", r.recipient_id] });
      navigate(`/runs/${r.run_id}`);
    },
  });

  const feedback = useMutation({
    mutationFn: (v: { bouquetId: number; liked: boolean }) =>
      call(
        api.POST("/recommendations/{id}/feedback", {
          params: { path: { id: runId } },
          body: { bouquet_id: v.bouquetId, liked: v.liked, rating: v.liked ? 5 : 2 },
        }),
      ),
    onSuccess: (_, v) =>
      setFeedbackSent((m) => ({
        ...m,
        [v.bouquetId]: v.liked ? "Учтём: понравилось" : "Учтём: не подходит",
      })),
  });

  const toggle = (f: Filter) => {
    if (filters.includes(f)) {
      setFilters(filters.filter((x) => x !== f));
      if (f === "cheaper") setCheaperThan(null);
      return;
    }
    if (f === "nearby" && !point) {
      // FR-L01: геолокация только по клику и с разрешения; не сохраняется
      if ("geolocation" in navigator) {
        navigator.geolocation.getCurrentPosition(
          (pos) => setPoint({ lat: pos.coords.latitude, lon: pos.coords.longitude }),
          () => setGeoNote("Геолокация недоступна — нажмите на карту, чтобы выбрать точку"),
          { timeout: 8000 },
        );
      } else {
        setGeoNote("Нажмите на карту, чтобы выбрать точку");
      }
    }
    if (f === "cheaper" && !cheaperThan && run.data?.items[0])
      setCheaperThan(run.data.items[0].bouquet_id);
    setFilters([...filters, f]);
  };

  if (run.isPending) return <Spinner label="Загружаю рекомендации…" />;
  if (run.error) return <ErrorState error={errorMessage(run.error)} />;
  const data = run.data;
  const q = data.clarifying_question;
  const cards = filters.length > 0 ? (offers.data?.items ?? []) : data.items;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <Link
            to={`/recipients/${data.recipient_id}`}
            className="text-sm text-brand-700 hover:underline"
          >
            ← К получателю
          </Link>
          <h1 className="text-2xl font-semibold">Подборка букетов</h1>
          <p className="text-sm text-stone-600">
            Исключено по ограничениям: {data.excluded_count} · модель {data.model_version} · правила{" "}
            {data.rules_version}
            {data.numerology_used && " · с нумерологией"}
          </p>
          <p className="text-xs text-stone-500">
            Повод <span data-testid="occasion-id">№{data.occasion_id}</span> — назовите номер
            флористу, чтобы он проверил замену цветов с учётом ограничений получателя (детали
            ограничений флорист не увидит).
          </p>
        </div>
      </div>

      {q && (
        <Card className="border-brand-200 bg-brand-50">
          <p className="font-medium" data-testid="clarifying-question">
            {q.text}
          </p>
          <p className="mt-1 text-xs text-stone-600">Один ответ заметно уточнит подбор.</p>
          <div className="mt-3 flex flex-wrap gap-2">
            {(quickAnswers[q.attribute] ?? []).map((a) => (
              <Button
                key={a.label}
                variant="secondary"
                disabled={answer.isPending}
                onClick={() => answer.mutate({ attribute: q.attribute, value: a.value })}
              >
                {a.label}
              </Button>
            ))}
            <Link
              to={`/recipients/${data.recipient_id}`}
              className="self-center text-sm text-brand-700 underline"
            >
              Указать в профиле
            </Link>
          </div>
          {answer.error && (
            <div className="mt-2">
              <ErrorState error={errorMessage(answer.error)} />
            </div>
          )}
        </Card>
      )}

      <div className="flex flex-wrap items-center gap-2" role="group" aria-label="Фильтры">
        {(Object.keys(filterLabels) as Filter[]).map((f) => (
          <button
            key={f}
            aria-pressed={filters.includes(f)}
            onClick={() => toggle(f)}
            className={`rounded-full border px-3 py-1.5 text-sm ${filters.includes(f) ? "border-brand-600 bg-brand-600 text-white" : "border-stone-300 bg-white hover:bg-stone-50"}`}
          >
            {filterLabels[f]}
          </button>
        ))}
        {filters.includes("cheaper") && cheaperThan && (
          <span className="text-xs text-stone-600">
            дешевле, чем «
            {data.items.find((i) => i.bouquet_id === cheaperThan)?.name ?? "выбранный"}»
          </span>
        )}
      </div>
      {geoNote && <p className="text-sm text-amber-700">{geoNote}</p>}

      <Suspense fallback={<Spinner label="Загружаю карту…" />}>
        {shops.data && (
          <ShopsMap
            shops={shops.data.items}
            point={point}
            onPick={(p) => {
              setPoint(p);
              setGeoNote(null);
            }}
          />
        )}
      </Suspense>

      {filters.length > 0 && offers.isFetching && <Spinner />}
      {offers.error && <ErrorState error={errorMessage(offers.error)} />}
      {filters.length > 0 && offers.data?.message && <EmptyState title={offers.data.message} />}
      {filters.length === 0 && data.items.length === 0 && (
        <EmptyState title="Подходящих букетов не нашлось">
          Попробуйте увеличить бюджет или уточнить профиль.
        </EmptyState>
      )}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {cards.map((c) => (
          <BouquetCard
            key={c.bouquet_id}
            card={c}
            actions={
              feedbackSent[c.bouquet_id] ? (
                <Badge tone="ok">{feedbackSent[c.bouquet_id]}</Badge>
              ) : (
                <>
                  <Button
                    variant="secondary"
                    onClick={() => feedback.mutate({ bouquetId: c.bouquet_id, liked: true })}
                  >
                    Нравится
                  </Button>
                  <Button
                    variant="ghost"
                    onClick={() => feedback.mutate({ bouquetId: c.bouquet_id, liked: false })}
                  >
                    Не то
                  </Button>
                  <Button
                    variant="ghost"
                    onClick={() => {
                      setCheaperThan(c.bouquet_id);
                      if (!filters.includes("cheaper")) setFilters([...filters, "cheaper"]);
                    }}
                  >
                    Дешевле
                  </Button>
                </>
              )
            }
          />
        ))}
      </div>

      {offers.data && offers.data.gifts.length > 0 && (
        <Card>
          <h2 className="mb-2 font-semibold">Подарки со скидкой</h2>
          <ul className="text-sm">
            {offers.data.gifts.map((g) => (
              <li key={g.id}>
                {g.name} — {g.discount_price} ₸ вместо {g.price} ₸
              </li>
            ))}
          </ul>
        </Card>
      )}

      {occasion.data && <GiftSets runId={runId} defaultBudget={occasion.data.budget_max} />}
    </div>
  );
}
