import type { Schemas } from "../../api/client";
import type { ProfileValue } from "./ProfileEditor";
import { FIELDS, type Confidence } from "./fields";

/** Профиль из ответа API в состояние формы; отсутствующее поле = «не знаю». */
export function profileFromApi(attrs: Schemas["Attribute"][] | undefined): ProfileValue {
  const byKey = new Map((attrs ?? []).map((a) => [a.key, a]));
  const out: ProfileValue = {};
  for (const f of FIELDS) {
    const a = byKey.get(f.key as Schemas["AttributeKey"]);
    if (a?.hidden) out[f.key] = { confidence: "unknown", value: null, hidden: true };
    else
      out[f.key] = {
        confidence: (a?.confidence ?? "unknown") as Confidence,
        value: a?.value ?? null,
        source: a?.source,
      };
  }
  return out;
}
