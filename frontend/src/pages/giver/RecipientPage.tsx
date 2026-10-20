import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, call, errorMessage, type Schemas } from "../../api/client";
import { Button, Card, EmptyState, ErrorState, Spinner } from "../../components/ui";
import { money } from "../../components/format";
import { ProfileEditor } from "../../features/profile/ProfileEditor";
import { profileFromApi } from "../../features/profile/fromApi";
import { OccasionForm } from "../../features/recommendations/OccasionForm";

export function RecipientPage() {
  const id = Number(useParams().id);
  const qc = useQueryClient();
  const navigate = useNavigate();
  const recipient = useQuery({
    queryKey: ["recipient", id],
    queryFn: () => call(api.GET("/recipients/{id}", { params: { path: { id } } })),
  });
  const runs = useQuery({
    queryKey: ["runs", id],
    queryFn: () => call(api.GET("/recipients/{id}/recommendations", { params: { path: { id } } })),
  });

  const save = useMutation({
    mutationFn: (attributes: Schemas["AttributeInput"][]) =>
      call(api.PUT("/recipients/{id}", { params: { path: { id } }, body: { attributes } })),
    onSuccess: (r) => qc.setQueryData(["recipient", id], r),
  });
  const remove = useMutation({
    mutationFn: () => call(api.DELETE("/recipients/{id}", { params: { path: { id } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["recipients"] });
      navigate("/recipients");
    },
  });

  if (recipient.isPending) return <Spinner />;
  if (recipient.error) return <ErrorState error={errorMessage(recipient.error)} />;
  const r = recipient.data;

  return (
    <div className="grid gap-6 lg:grid-cols-[1fr_1fr]">
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-semibold">{r.nickname}</h1>
          <Button
            variant="danger"
            onClick={() => {
              if (confirm("Удалить получателя вместе с профилем, поводами и датой рождения?"))
                remove.mutate();
            }}
          >
            Удалить
          </Button>
        </div>
        <Card>
          <h2 className="mb-1 font-semibold">Профиль</h2>
          <p className="mb-3 text-sm text-stone-600">
            «Не знаю» — это нормально: система будет осторожнее и подскажет, что стоит уточнить.
          </p>
          <ProfileEditor
            key={r.updated_at}
            initial={profileFromApi(r.attributes)}
            saving={save.isPending}
            onSave={(changed) =>
              save.mutate(
                changed.map((c) => ({
                  key: c.key as Schemas["AttributeKey"],
                  value: c.value,
                  confidence: c.confidence,
                  source: "giver",
                })),
              )
            }
          />
          {save.error && (
            <div className="mt-2">
              <ErrorState error={errorMessage(save.error)} />
            </div>
          )}
          {save.isSuccess && <p className="mt-2 text-sm text-emerald-700">Профиль сохранён</p>}
        </Card>
      </div>
      <div className="space-y-4">
        <Card>
          <h2 className="mb-3 font-semibold">Новый повод</h2>
          <OccasionForm recipientId={id} onDone={(runId) => navigate(`/runs/${runId}`)} />
        </Card>
        <Card>
          <h2 className="mb-3 font-semibold">История рекомендаций</h2>
          {runs.isPending && <Spinner />}
          {runs.data?.items.length === 0 && <EmptyState title="Рекомендаций пока не было" />}
          <ul className="divide-y divide-stone-100">
            {runs.data?.items.map((run) => (
              <li key={run.run_id} className="flex items-center justify-between py-2 text-sm">
                <Link className="text-brand-700 hover:underline" to={`/runs/${run.run_id}`}>
                  {new Date(run.created_at).toLocaleString("ru-RU")} — {run.items.length} вариантов
                </Link>
                <span className="text-stone-500">
                  {run.items[0]
                    ? `от ${money(Math.min(...run.items.map((i) => i.discount_price ?? i.price)))}`
                    : ""}
                </span>
              </li>
            ))}
          </ul>
        </Card>
      </div>
    </div>
  );
}
