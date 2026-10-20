import type { components } from "../../api/schema";

export type Session = components["schemas"]["AuthResponse"];

const KEY = "gulder.session";
type Listener = (s: Session | null) => void;
const listeners = new Set<Listener>();

function read(): Session | null {
  try {
    const raw = localStorage.getItem(KEY);
    return raw ? (JSON.parse(raw) as Session) : null;
  } catch {
    return null;
  }
}

let current: Session | null = read();

/** Хранилище сессии (ADR 0008): токены в localStorage, подписчики узнают о входе и выходе. */
export const authStore = {
  get: () => current,
  set(s: Session) {
    current = s;
    try {
      localStorage.setItem(KEY, JSON.stringify(s));
    } catch {
      /* приватный режим — сессия живёт до перезагрузки */
    }
    listeners.forEach((l) => l(s));
  },
  clear() {
    current = null;
    try {
      localStorage.removeItem(KEY);
    } catch {
      /* ignore */
    }
    listeners.forEach((l) => l(null));
  },
  subscribe(l: Listener) {
    listeners.add(l);
    return () => listeners.delete(l);
  },
};
