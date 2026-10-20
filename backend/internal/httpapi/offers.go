package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/zbekxzz/gulder/backend/internal/offers"
	"github.com/zbekxzz/gulder/backend/internal/recommend"
)

// getOffers — GET /offers?filter=nearby,cheaper,discount&lat&lon&run_id&bouquet_id (§10).
// lat/lon используются только в памяти запроса: не пишутся в БД, логи и audit (FR-L01).
func (s *Server) getOffers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filters, err := offers.ParseFilters(q.Get("filter"))
	if err != nil {
		writeError(w, r, errValidation(map[string]any{"filter": err.Error()}))
		return
	}
	runID, err := strconv.ParseInt(q.Get("run_id"), 10, 64)
	if err != nil || runID <= 0 {
		writeError(w, r, errValidation(map[string]any{"run_id": "обязательный параметр"}))
		return
	}
	req := offers.Request{Filters: filters}
	if q.Has("lat") || q.Has("lon") {
		lat, err1 := strconv.ParseFloat(q.Get("lat"), 64)
		lon, err2 := strconv.ParseFloat(q.Get("lon"), 64)
		if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			writeError(w, r, errValidation(map[string]any{"lat": "координаты вне диапазона"}))
			return
		}
		req.Point = &offers.Point{Lat: lat, Lon: lon}
	}
	if v := q.Get("bouquet_id"); v != "" {
		if req.BouquetID, err = strconv.ParseInt(v, 10, 64); err != nil {
			writeError(w, r, errValidation(map[string]any{"bouquet_id": "ожидается число"}))
			return
		}
	}
	giver := principal(r).UserID
	occasionID, _, err := recommend.RunOwner(r.Context(), s.store.Pool, giver, runID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	// базовое множество — Rules Engine для текущего профиля получателя (FR-L05)
	c, err := recommend.Build(r.Context(), s.store.Pool, s.ranking, giver, occasionID, s.now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	resp, err := offers.Apply(c, req)
	switch {
	case errors.Is(err, offers.ErrNeedPoint):
		writeError(w, r, errValidation(map[string]any{"lat": err.Error()}))
	case errors.Is(err, offers.ErrNeedBouquet), errors.Is(err, offers.ErrUnknownCurrent):
		writeError(w, r, errValidation(map[string]any{"bouquet_id": err.Error()}))
	case err != nil:
		writeError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, resp)
	}
}
