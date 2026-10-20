export type Role = "giver" | "florist" | "manager";

/** Стартовая страница роли. */
export function homeFor(role: Role): string {
  return { giver: "/recipients", florist: "/florist", manager: "/manager" }[role];
}
