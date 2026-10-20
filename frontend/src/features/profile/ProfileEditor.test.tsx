import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, test, vi } from "vitest";
import { ProfileEditor } from "./ProfileEditor";
import { profileFromApi } from "./fromApi";

describe("ProfileEditor", () => {
  test("AC-2: «Не знаю» — отдельная опция, значение не вводится и уходит как null", () => {
    const onSave = vi.fn();
    render(<ProfileEditor initial={profileFromApi([])} onSave={onSave} />);
    const allergies = screen.getByRole("group", { name: "Аллергии" });
    // по умолчанию поле неизвестно — выбора аллергенов нет
    expect(within(allergies).getByRole("radio", { name: "Не знаю" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    expect(within(allergies).queryByRole("button", { name: "Орехи" })).not.toBeInTheDocument();

    fireEvent.click(within(allergies).getByRole("radio", { name: "Знаю" }));
    expect(within(allergies).getByText(/Аллергий нет/)).toBeInTheDocument();
    fireEvent.click(within(allergies).getByRole("button", { name: "Орехи" }));
    fireEvent.click(within(allergies).getByRole("radio", { name: "Не знаю" }));
    fireEvent.click(screen.getByRole("button", { name: "Сохранить профиль" }));
    expect(onSave).toHaveBeenCalledWith([{ key: "allergies", confidence: "unknown", value: null }]);
  });

  test("«Знаю» с пустым списком — явное «нет», а не пропуск", () => {
    const onSave = vi.fn();
    render(<ProfileEditor initial={profileFromApi([])} onSave={onSave} />);
    const pets = screen.getByRole("group", { name: "Домашние животные" });
    fireEvent.click(within(pets).getByRole("radio", { name: "Знаю" }));
    fireEvent.click(screen.getByRole("button", { name: "Сохранить профиль" }));
    expect(onSave).toHaveBeenCalledWith([{ key: "pets", confidence: "know", value: [] }]);
  });

  test("поля, заполненные получателем, показаны скрытыми", () => {
    render(
      <ProfileEditor
        initial={profileFromApi([{ key: "allergies", hidden: true }])}
        onSave={() => {}}
      />,
    );
    expect(screen.getByText("заполнено получателем, скрыто")).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "Аллергии" })).not.toBeInTheDocument();
  });

  test("дата при «знаю» обязательна — сохранить нельзя без значения", () => {
    render(<ProfileEditor initial={profileFromApi([])} onSave={() => {}} />);
    const birth = screen.getByRole("group", { name: /Дата рождения/ });
    fireEvent.click(within(birth).getByRole("radio", { name: "Знаю" }));
    expect(screen.getByRole("button", { name: "Сохранить профиль" })).toBeDisabled();
  });
});
