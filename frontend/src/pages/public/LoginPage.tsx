import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { Link, useNavigate } from "react-router-dom";
import { z } from "zod";
import { errorMessage } from "../../api/client";
import { Button, Card, ErrorState, Field, Input } from "../../components/ui";
import { useAuth } from "../../features/auth/AuthContext";
import { homeFor } from "../../routing";

const schema = z.object({
  email: z.string().min(1, "Введите email"),
  password: z.string().min(1, "Введите пароль"),
  displayName: z.string().max(100).optional(),
});
type Form = z.infer<typeof schema>;

export function AuthPage({ mode }: { mode: "login" | "register" }) {
  const { login, register: signUp } = useAuth();
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);
  const { register, handleSubmit, formState } = useForm<Form>({ resolver: zodResolver(schema) });

  const onSubmit = handleSubmit(async (f) => {
    setError(null);
    try {
      const s =
        mode === "login"
          ? await login(f.email, f.password)
          : await signUp(f.email, f.password, f.displayName ?? "");
      navigate(homeFor(s.user.role), { replace: true });
    } catch (e) {
      setError(errorMessage(e));
    }
  });

  return (
    <div className="mx-auto mt-10 max-w-sm">
      <Card>
        <h1 className="mb-1 text-xl font-semibold">
          {mode === "login" ? "Вход" : "Регистрация дарителя"}
        </h1>
        <p className="mb-4 text-sm text-stone-600">
          Подбор букета и подарков с учётом аллергий, питомцев и бюджета.
        </p>
        <form className="space-y-3" onSubmit={onSubmit} noValidate>
          <Field label="Email" error={formState.errors.email?.message}>
            <Input type="email" autoComplete="email" {...register("email")} />
          </Field>
          <Field
            label="Пароль"
            error={formState.errors.password?.message}
            hint={mode === "register" ? "Не меньше 8 символов" : undefined}
          >
            <Input
              type="password"
              autoComplete={mode === "login" ? "current-password" : "new-password"}
              {...register("password")}
            />
          </Field>
          {mode === "register" && (
            <Field label="Как к вам обращаться">
              <Input {...register("displayName")} />
            </Field>
          )}
          {error && <ErrorState error={error} />}
          <Button type="submit" className="w-full" disabled={formState.isSubmitting}>
            {mode === "login" ? "Войти" : "Зарегистрироваться"}
          </Button>
        </form>
        <p className="mt-4 text-center text-sm text-stone-600">
          {mode === "login" ? (
            <>
              Нет аккаунта?{" "}
              <Link className="text-brand-700 underline" to="/register">
                Регистрация
              </Link>
            </>
          ) : (
            <>
              Уже есть аккаунт?{" "}
              <Link className="text-brand-700 underline" to="/login">
                Войти
              </Link>
            </>
          )}
        </p>
      </Card>
    </div>
  );
}
