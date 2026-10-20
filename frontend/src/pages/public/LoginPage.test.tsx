import { fireEvent, screen, waitFor } from "@testing-library/react";
import { expect, test } from "vitest";
import { AuthProvider } from "../../features/auth/AuthContext";
import { authStore } from "../../features/auth/store";
import { mockFetch, renderWithProviders } from "../../test/utils";
import { AuthPage } from "./LoginPage";

test("вход отправляет email и пароль и сохраняет сессию", async () => {
  let sent: unknown;
  mockFetch((_m, path, body) => {
    sent = body;
    if (path === "/api/v1/auth/login")
      return [
        200,
        {
          access_token: "a.b.c",
          refresh_token: "r",
          expires_in: 900,
          user: { id: 1, email: "g@x", display_name: "", role: "giver", shop_id: null },
        },
      ];
    return [404, {}];
  });
  renderWithProviders(
    <AuthProvider>
      <AuthPage mode="login" />
    </AuthProvider>,
  );
  fireEvent.change(screen.getByLabelText("Email"), { target: { value: "g@x" } });
  fireEvent.change(screen.getByLabelText("Пароль"), { target: { value: "secret-pass" } });
  fireEvent.click(screen.getByRole("button", { name: "Войти" }));
  await waitFor(() => expect(authStore.get()?.user.email).toBe("g@x"));
  expect(sent).toEqual({ email: "g@x", password: "secret-pass" });
  authStore.clear();
});
