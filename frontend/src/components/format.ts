/** Сумма в тенге для отображения. Арифметики над ценами на клиенте нет (ADR 0002). */
export function money(v: number | null | undefined): string {
  if (v === null || v === undefined) return "—";
  return `${new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 2 }).format(v)} ₸`;
}
