import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { api, call, errorMessage } from "../../api/client";
import { Badge, Button, Card, ErrorState, Spinner } from "../../components/ui";
import { useAuth } from "../../features/auth/AuthContext";

/** Настройки дарителя: исследовательский режим (RM-01, RM-02) и удаление аккаунта (§18.5). */
export function SettingsPage() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const { logout } = useAuth();
  const consent = useQuery({
    queryKey: ["consent"],
    queryFn: () => call(api.GET("/research/consent")),
  });
  const grant = useMutation({
    mutationFn: (version: string) =>
      call(api.POST("/research/consent", { body: { consent_version: version } })),
    onSuccess: (s) => qc.setQueryData(["consent"], s),
  });
  const revoke = useMutation({
    mutationFn: () => call(api.DELETE("/research/consent")),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["consent"] }),
  });
  const removeAccount = useMutation({
    mutationFn: () => call(api.DELETE("/me")),
    onSuccess: () => {
      logout();
      navigate("/login");
    },
  });

  if (consent.isPending) return <Spinner />;
  if (consent.error) return <ErrorState error={errorMessage(consent.error)} />;
  const s = consent.data;

  return (
    <div className="max-w-2xl space-y-4">
      <h1 className="text-2xl font-semibold">Настройки</h1>
      <Card>
        <div className="mb-2 flex items-center justify-between gap-2">
          <h2 className="font-semibold">Исследовательский режим</h2>
          <Badge tone={s.active ? "ok" : "neutral"}>{s.active ? "включён" : "выключен"}</Badge>
        </div>
        <p className="whitespace-pre-line text-sm text-stone-700">{s.text}</p>
        <p className="mt-2 text-xs text-stone-500">
          Версия текста: {s.current_version}
          {s.active &&
            s.granted_at &&
            ` · согласие от ${new Date(s.granted_at).toLocaleDateString("ru-RU")} (версия ${s.version})`}
        </p>
        <div className="mt-3">
          {s.active ? (
            <Button variant="danger" onClick={() => revoke.mutate()} disabled={revoke.isPending}>
              Отозвать согласие
            </Button>
          ) : (
            <Button onClick={() => grant.mutate(s.current_version)} disabled={grant.isPending}>
              Согласен участвовать
            </Button>
          )}
        </div>
        {(grant.error || revoke.error) && (
          <div className="mt-2">
            <ErrorState error={errorMessage(grant.error ?? revoke.error)} />
          </div>
        )}
      </Card>
      <Card>
        <h2 className="mb-2 font-semibold">Удаление аккаунта</h2>
        <p className="mb-3 text-sm text-stone-700">
          Удалятся все получатели, их профили (включая даты рождения), поводы, рекомендации и
          отзывы, а также связь с исследовательским набором. Действие необратимо.
        </p>
        <Button
          variant="danger"
          disabled={removeAccount.isPending}
          onClick={() => {
            if (confirm("Удалить аккаунт и все данные без возможности восстановления?"))
              removeAccount.mutate();
          }}
        >
          Удалить аккаунт
        </Button>
        {removeAccount.error && (
          <div className="mt-2">
            <ErrorState error={errorMessage(removeAccount.error)} />
          </div>
        )}
      </Card>
    </div>
  );
}
