import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";

export function renderWithProviders(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

/** Подменяет fetch: обработчик получает метод и путь, возвращает [status, body]. */
export function mockFetch(
  handler: (method: string, path: string, body: unknown) => [number, unknown],
) {
  const fn = async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = input instanceof Request ? input : new Request(String(input), init);
    const url = new URL(req.url, "http://localhost");
    const text = req.method === "GET" ? "" : await req.text();
    const [status, body] = handler(
      req.method,
      url.pathname + url.search,
      text ? JSON.parse(text) : undefined,
    );
    return new Response(status === 204 ? null : JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  };
  globalThis.fetch = fn as typeof fetch;
}
