import "leaflet/dist/leaflet.css";
import { CircleMarker, MapContainer, Popup, TileLayer, useMapEvents } from "react-leaflet";
import type { Schemas } from "../../api/client";

export type Point = { lat: number; lon: number };

const ALMATY: [number, number] = [43.238, 76.92];
const days: [string, string][] = [
  ["mon", "Пн"],
  ["tue", "Вт"],
  ["wed", "Ср"],
  ["thu", "Чт"],
  ["fri", "Пт"],
  ["sat", "Сб"],
  ["sun", "Вс"],
];

function ClickToPick({ onPick }: { onPick?: (p: Point) => void }) {
  useMapEvents({ click: (e) => onPick?.({ lat: e.latlng.lat, lon: e.latlng.lng }) });
  return null;
}

/** Карта магазинов (FR-L02): OpenStreetMap через Leaflet. Точка дарителя живёт только в памяти страницы. */
export function ShopsMap({
  shops,
  point,
  onPick,
}: {
  shops: Schemas["Shop"][];
  point?: Point | null;
  onPick?: (p: Point) => void;
}) {
  return (
    <div
      className="h-72 overflow-hidden rounded-xl border border-stone-200"
      data-testid="shops-map"
    >
      <MapContainer
        center={point ? [point.lat, point.lon] : ALMATY}
        zoom={12}
        className="h-full w-full"
        scrollWheelZoom={false}
      >
        <TileLayer
          attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
          url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
        />
        <ClickToPick onPick={onPick} />
        {shops.map((s) => (
          <CircleMarker
            key={s.id}
            center={[s.lat, s.lon]}
            radius={9}
            pathOptions={{ color: s.open_now ? "#db2777" : "#78716c", fillOpacity: 0.7 }}
          >
            <Popup>
              <strong>{s.name}</strong>
              <br />
              {s.address}
              <br />
              {s.open_now ? "Открыто сейчас" : "Сейчас закрыто"}
              <ul style={{ marginTop: 4 }}>
                {days.map(([k, l]) => (
                  <li key={k}>
                    {l}:{" "}
                    {(s.opening_hours[k] ?? []).map((iv) => iv.join("–")).join(", ") || "выходной"}
                  </li>
                ))}
              </ul>
            </Popup>
          </CircleMarker>
        ))}
        {point && (
          <CircleMarker
            center={[point.lat, point.lon]}
            radius={7}
            pathOptions={{ color: "#0284c7", fillOpacity: 1 }}
          />
        )}
      </MapContainer>
    </div>
  );
}
