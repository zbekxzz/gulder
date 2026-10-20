import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((f: string) => {
    const p = join(dir, f);
    if (statSync(p).isDirectory()) return files(p);
    return /\.(tsx?|ts)$/.test(p) && !p.includes(".test.") && !p.endsWith("schema.d.ts") ? [p] : [];
  });
}

// §15, DoD: никаких «безопасно» / «гарантированно» в текстах интерфейса
test("в UI нет обещаний безопасности", () => {
  const offenders = files("src").filter((f) => /безопасн|гарантир/i.test(readFileSync(f, "utf8")));
  expect(offenders).toEqual([]);
});
