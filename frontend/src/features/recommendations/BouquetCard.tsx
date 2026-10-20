import type { ReactNode } from "react";
import type { Schemas } from "../../api/client";
import { Disclaimer } from "../../components/Disclaimer";
import { Badge, Card, Meter } from "../../components/ui";
import { money } from "../../components/format";
import { colors, label, missingLabels, petsShort, scentLevels } from "../labels";

type CardData = Schemas["RecommendationCard"] & { price_diff?: number };

/** Карточка Top-5 (§8.2, §15): причины, конфликты, недостающие сведения, предупреждения, состав. */
export function BouquetCard({ card, actions }: { card: CardData; actions?: ReactNode }) {
  return (
    <Card className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-2">
        <div>
          <h3 className="font-semibold" data-testid="card-name">
            {card.rank > 0 && <span className="mr-1 text-stone-400">#{card.rank}</span>}
            {card.name}
          </h3>
          <p className="text-xs text-stone-500">
            {card.shop.name}
            {card.shop.distance_m != null && ` · ${(card.shop.distance_m / 1000).toFixed(1)} км`}
            {" · "}
            {card.shop.open_now ? "открыто сейчас" : "закрыто"}
          </p>
        </div>
        <div className="whitespace-nowrap text-right">
          {card.discount_price != null ? (
            <>
              <div className="font-semibold text-brand-700">{money(card.discount_price)}</div>
              <div className="text-xs text-stone-400 line-through">{money(card.price)}</div>
              {card.discount_until && (
                <div className="text-xs text-stone-500">
                  до {new Date(card.discount_until).toLocaleDateString("ru-RU")}
                </div>
              )}
            </>
          ) : (
            <div className="font-semibold">{money(card.price)}</div>
          )}
          {card.price_diff != null && <Badge tone="ok">дешевле на {money(card.price_diff)}</Badge>}
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <Meter label="Соответствие" value={card.score} />
        <Meter label="Уверенность" value={card.confidence} />
      </div>

      {card.numerology && (
        <div className="rounded-lg bg-violet-50 p-2 text-xs text-violet-900">
          Число {card.numerology.number}: {card.numerology.meaning}{" "}
          <Badge tone="info">{card.numerology.label}</Badge>
        </div>
      )}

      {card.reasons.length > 0 && <List title="Почему подходит" items={card.reasons} />}
      {card.conflicts.length > 0 && (
        <List title="Что может не понравиться" items={card.conflicts} tone="text-amber-800" />
      )}
      {card.warnings.length > 0 && (
        <List
          title="Предупреждения"
          items={card.warnings.map((w) => w.message)}
          tone="text-red-800"
        />
      )}
      {card.missing_info.length > 0 && (
        <p className="text-xs text-stone-600">
          Не хватает сведений: {card.missing_info.map((m) => label(missingLabels, m)).join(", ")}
        </p>
      )}

      <details className="text-sm">
        <summary className="cursor-pointer text-stone-700">Состав букета</summary>
        <ul className="mt-2 space-y-1">
          {card.items.map((it) => (
            <li key={it.flower_id} className="flex flex-wrap items-center gap-1.5 text-xs">
              <span className="font-medium">{it.trade_name}</span>
              <span className="text-stone-500">× {it.stems}</span>
              <span className="text-stone-500">{label(colors, it.color)}</span>
              <Badge>{label(scentLevels, it.scent)}</Badge>
              {it.toxic_for.length > 0 && (
                <Badge tone="warn">
                  токсичен для {it.toxic_for.map((p) => label(petsShort, p)).join(", ")}
                </Badge>
              )}
              {it.image_url && (
                <a
                  className="text-brand-700 underline"
                  href={it.image_url}
                  target="_blank"
                  rel="noreferrer"
                >
                  фото ({it.image_license}
                  {it.image_author ? `, ${it.image_author}` : ""})
                </a>
              )}
            </li>
          ))}
        </ul>
      </details>
      <Disclaimer />
      {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
    </Card>
  );
}

function List({
  title,
  items,
  tone = "text-stone-700",
}: {
  title: string;
  items: string[];
  tone?: string;
}) {
  return (
    <div className={`text-sm ${tone}`}>
      <p className="text-xs font-medium uppercase tracking-wide text-stone-500">{title}</p>
      <ul className="mt-1 list-disc space-y-0.5 pl-5">
        {items.map((i) => (
          <li key={i}>{i}</li>
        ))}
      </ul>
    </div>
  );
}
