import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, test } from "vitest";
import { mockFetch, renderWithProviders } from "../../test/utils";
import { SubstitutionsTab } from "./FloristPage";

const bouquets = {
  items: [
    {
      id: 10,
      sku: "AB-001",
      name: "Розы",
      packaging: "",
      style: "classic",
      composition_format: "bouquet",
      image_url: "",
      active: true,
      availability: { id: 1, in_stock: true, quantity: null },
      offer: { price: 12000, discount_price: null, discount_from: null, discount_until: null },
      items: [
        {
          flower_id: 1,
          code: "rose_red",
          trade_name: "Роза",
          color: "red",
          genus: "Rosa",
          stems: 9,
          taxon_confirmed: true,
        },
      ],
    },
  ],
};
const flowers = {
  items: [
    {
      id: 2,
      code: "lily",
      trade_name: "Лилия",
      color: "white",
      scent: "strong",
      taxon_id: 2,
      genus: "Lilium",
      taxon_confirmed: true,
      shared: true,
    },
    {
      id: 3,
      code: "gerbera",
      trade_name: "Гербера",
      color: "pink",
      scent: "none",
      taxon_id: 3,
      genus: "Gerbera",
      taxon_confirmed: true,
      shared: true,
    },
  ],
};

beforeEach(() => {
  mockFetch((_method, path, body) => {
    if (path.startsWith("/api/v1/catalog/bouquets")) return [200, bouquets];
    if (path.startsWith("/api/v1/catalog/flowers")) return [200, flowers];
    if (path === "/api/v1/bouquets/10/substitutions/validate") {
      const to = (body as { to_flower_id: number }).to_flower_id;
      if (to === 2)
        return [
          409,
          {
            error: {
              code: "substitution_blocked",
              message: "Замена запрещена",
              details: {
                decision: "block",
                giver_view: {
                  suitable: false,
                  decision: "block",
                  missing_info: [],
                  reasons: [{ code: "PET_TOXIC", message: "«Лилия» опасна для животного" }],
                },
              },
            },
          },
        ];
      return [
        200,
        {
          decision: "allow",
          giver_view: { suitable: true, decision: "allow", reasons: [], missing_info: [] },
          composition: [],
        },
      ];
    }
    return [404, {}];
  });
});

test("AC-3 (UI): опасная замена — кнопка неактивна и показана причина; безопасная — активна", async () => {
  renderWithProviders(<SubstitutionsTab />);
  fireEvent.change(await screen.findByRole("combobox", { name: "Букет" }), {
    target: { value: "10" },
  });
  fireEvent.change(screen.getByRole("combobox", { name: "Заменить цветок" }), {
    target: { value: "1" },
  });
  fireEvent.change(screen.getByRole("combobox", { name: "На цветок" }), { target: { value: "2" } });

  expect(await screen.findByText("Замена запрещена")).toBeInTheDocument();
  expect(screen.getByText("«Лилия» опасна для животного")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Применить замену" })).toBeDisabled();

  fireEvent.change(screen.getByRole("combobox", { name: "На цветок" }), { target: { value: "3" } });
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Применить замену" })).toBeEnabled(),
  );
  expect(screen.getByText("Известных противопоказаний не найдено")).toBeInTheDocument();
});
