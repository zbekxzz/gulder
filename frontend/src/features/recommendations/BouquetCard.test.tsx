import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import type { Schemas } from "../../api/client";
import { BouquetCard } from "./BouquetCard";

const card: Schemas["RecommendationCard"] = {
  bouquet_id: 1,
  rank: 1,
  name: "Нежность",
  style: "classic",
  composition_format: "bouquet",
  score: 0.81,
  confidence: 0.64,
  reasons: ["Любимые цвета: белый"],
  conflicts: ["Дешевле минимального бюджета"],
  missing_info: ["pets"],
  warnings: [{ code: "RECIPIENT_RESTRICTION", message: "Не подходит по ограничениям получателя" }],
  numerology: { number: 7, meaning: "Мудрость", label: "развлекательная функция" },
  shop: { id: 1, name: "Гүлдер на Абая", address: "Абая, 52", distance_m: null, open_now: true },
  price: 15000,
  discount_price: 12900,
  discount_until: "2026-11-01T00:00:00Z",
  in_stock: true,
  items: [
    {
      flower_id: 1,
      trade_name: "Хризантема",
      color: "white",
      stems: 5,
      genus: "Chrysanthemum",
      scent: "low",
      toxic_for: ["cat", "dog"],
    },
  ],
};

test("FR-R03: карточка показывает причины, конфликты, недостающие сведения, нумерологию и дисклеймер", () => {
  render(<BouquetCard card={card} />);
  expect(screen.getByText("Любимые цвета: белый")).toBeInTheDocument();
  expect(screen.getByText("Дешевле минимального бюджета")).toBeInTheDocument();
  expect(screen.getByText(/Не хватает сведений: домашние животные/)).toBeInTheDocument();
  expect(screen.getByText("Не подходит по ограничениям получателя")).toBeInTheDocument();
  expect(screen.getByText("развлекательная функция")).toBeInTheDocument();
  expect(
    screen.getByText(/Известных противопоказаний по указанным сведениям не найдено/),
  ).toBeInTheDocument();
  expect(screen.getByText("токсичен для кошек, собак")).toBeInTheDocument();
  expect(screen.getByRole("meter", { name: "Уверенность" })).toHaveAttribute("aria-valuenow", "64");
  expect(document.body.textContent).not.toMatch(/безопасн|гарантир/i);
});
