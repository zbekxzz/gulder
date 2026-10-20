import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, call, errorMessage } from "../../api/client";
import { Button, Card, EmptyState, ErrorState, Input, Spinner } from "../../components/ui";

export function RecipientsPage() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [nickname, setNickname] = useState("");
  const list = useQuery({ queryKey: ["recipients"], queryFn: () => call(api.GET("/recipients")) });
  const create = useMutation({
    mutationFn: (name: string) =>
      call(api.POST("/recipients", { body: { nickname: name, attributes: [] } })),
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: ["recipients"] });
      navigate(`/recipients/${r.id}`);
    },
  });

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">Получатели</h1>
      <Card>
        <form
          className="flex flex-col gap-2 sm:flex-row"
          onSubmit={(e) => {
            e.preventDefault();
            if (nickname.trim()) create.mutate(nickname.trim());
          }}
        >
          <Input
            placeholder="Например: Мама, Айгерим"
            value={nickname}
            onChange={(e) => setNickname(e.target.value)}
            aria-label="Имя получателя"
          />
          <Button type="submit" disabled={!nickname.trim() || create.isPending}>
            Добавить получателя
          </Button>
        </form>
        {create.error && (
          <div className="mt-2">
            <ErrorState error={errorMessage(create.error)} />
          </div>
        )}
      </Card>
      {list.isPending && <Spinner />}
      {list.error && <ErrorState error={errorMessage(list.error)} />}
      {list.data?.items.length === 0 && (
        <EmptyState title="Пока нет получателей">
          Добавьте человека, которому выбираете подарок.
        </EmptyState>
      )}
      <ul className="grid gap-3 sm:grid-cols-2">
        {list.data?.items.map((r) => (
          <li key={r.id}>
            <Link
              to={`/recipients/${r.id}`}
              className="block rounded-xl border border-stone-200 bg-white p-4 hover:border-brand-500"
            >
              <span className="font-medium">{r.nickname}</span>
              <span className="block text-xs text-stone-500">
                Обновлён {new Date(r.updated_at).toLocaleDateString("ru-RU")}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
