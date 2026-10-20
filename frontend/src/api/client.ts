import createClient, { type Middleware } from "openapi-fetch";
import type { components, paths } from "./schema";
import { authStore } from "../features/auth/store";

export type Schemas = components["schemas"];

/** Ошибка API в формате {"error":{code,message,details}} (§14). */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: Record<string, unknown>;

  constructor(
    status: number,
    code: string,
    message: string,
    details: Record<string, unknown> = {},
  ) {
    super(message);
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

// абсолютный адрес: в браузере совпадает с относительным, а в тестах (Node) Request требует полный URL
const BASE = `${globalThis.location?.origin ?? "http://localhost"}/api/v1`;

/** Секунд до истечения JWT (по полю exp); null — токен не разобрать. */
function secondsLeft(token: string): number | null {
  try {
    const payload = JSON.parse(atob(token.split(".")[1]!.replace(/-/g, "+").replace(/_/g, "/")));
    return typeof payload.exp === "number" ? payload.exp - Date.now() / 1000 : null;
  } catch {
    return null;
  }
}

let refreshing: Promise<void> | null = null;

/** Обновляет access-токен заранее, за минуту до истечения: так не нужно повторять запросы после 401. */
async function ensureFreshToken(): Promise<void> {
  const session = authStore.get();
  if (!session) return;
  const left = secondsLeft(session.access_token);
  if (left !== null && left > 60) return;
  refreshing ??= fetch(`${BASE}/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: session.refresh_token }),
  })
    .then(async (res) => {
      if (!res.ok) {
        authStore.clear();
        return;
      }
      authStore.set(await res.json());
    })
    .finally(() => {
      refreshing = null;
    });
  await refreshing;
}

const authMiddleware: Middleware = {
  async onRequest({ request, schemaPath }) {
    if (schemaPath.startsWith("/auth/")) return request;
    await ensureFreshToken();
    const session = authStore.get();
    if (session) request.headers.set("Authorization", `Bearer ${session.access_token}`);
    return request;
  },
  onResponse({ response }) {
    if (response.status === 401 && authStore.get()) authStore.clear();
    return response;
  },
};

// fetch берётся в момент запроса, а не при создании клиента: так его можно подменить в тестах
export const api = createClient<paths>({ baseUrl: BASE, fetch: (req) => globalThis.fetch(req) });
api.use(authMiddleware);

type Result<T> = { data?: T; error?: unknown; response: Response };

/** Возвращает данные или бросает ApiError. */
export async function call<T>(promise: Promise<Result<T>>): Promise<T> {
  const { data, error, response } = await promise;
  if (response.ok) return data as T;
  const body = (error ?? {}) as {
    error?: { code?: string; message?: string; details?: Record<string, unknown> };
  };
  throw new ApiError(
    response.status,
    body.error?.code ?? "http_error",
    body.error?.message ?? `Ошибка ${response.status}`,
    body.error?.details ?? {},
  );
}

/** Человекочитаемое сообщение об ошибке. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    const details = Object.entries(err.details)
      .filter(([, v]) => typeof v === "string")
      .map(([k, v]) => `${k}: ${v as string}`);
    return details.length ? `${err.message} (${details.join("; ")})` : err.message;
  }
  return err instanceof Error ? err.message : "Неизвестная ошибка";
}
