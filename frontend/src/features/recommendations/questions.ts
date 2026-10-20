/** Быстрые ответы на уточняющий вопрос (FR-R04, §15): ответ сразу обновляет профиль и пересчитывает Top-5. */
export const quickAnswers: Record<string, { label: string; value: unknown }[]> = {
  allergies: [{ label: "Аллергий нет", value: [] }],
  pets: [
    { label: "Животных нет", value: [] },
    { label: "Кошка", value: ["cat"] },
    { label: "Собака", value: ["dog"] },
    { label: "Кошка и собака", value: ["cat", "dog"] },
  ],
  asthma: [
    { label: "Астмы нет", value: false },
    { label: "Есть астма", value: true },
  ],
  scent_sensitivity: [
    { label: "Запахи не мешают", value: "none" },
    { label: "Слабо реагирует", value: "low" },
    { label: "Средне", value: "medium" },
    { label: "Сильные запахи не переносит", value: "strong" },
  ],
};
