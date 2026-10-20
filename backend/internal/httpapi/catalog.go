package httpapi

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/audit"
	"github.com/zbekxzz/gulder/backend/internal/catalog"
)

// importCatalog — POST /catalog/import?kind=flowers|bouquets|gifts, multipart поле file (§12).
func (s *Server) importCatalog(w http.ResponseWriter, r *http.Request) {
	shopID, err := floristShop(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	kind := r.URL.Query().Get("kind")
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if err := r.ParseMultipartForm(5 << 20); err != nil { //nolint:gosec // тело ограничено MaxBytesReader выше
		writeError(w, r, errBadRequest("Ожидается multipart/form-data с полем file (до 5 МБ)", nil))
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, errBadRequest("Нет файла в поле file", nil))
		return
	}
	defer func() { _ = file.Close() }()

	var rep catalog.ImportReport
	err = s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		var err error
		rep, err = catalog.Import(r.Context(), tx, shopID, kind, file)
		if err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, principal(r).UserID, audit.CatalogImported, "shop", shopID, map[string]any{
			"kind": kind, "imported": rep.Imported, "updated": rep.Updated, "errors": len(rep.Errors),
		})
	})
	var formatErr *catalog.FormatError
	switch {
	case errors.Is(err, catalog.ErrUnknownKind):
		writeError(w, r, errValidation(map[string]any{"kind": []string{catalog.KindFlowers, catalog.KindBouquets, catalog.KindGifts}}))
	case errors.As(err, &formatErr):
		writeError(w, r, errBadRequest("Некорректный CSV: "+formatErr.Error(), nil))
	case err != nil:
		writeError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, rep)
	}
}

func (s *Server) listShopBouquets(w http.ResponseWriter, r *http.Request) {
	shopID, err := floristShop(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := catalog.ListShopBouquets(r.Context(), s.store.Pool, shopID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": nonNilSlice(items)})
}

func (s *Server) listShopGifts(w http.ResponseWriter, r *http.Request) {
	shopID, err := floristShop(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := catalog.ListShopGifts(r.Context(), s.store.Pool, shopID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": nonNilSlice(items)})
}

func (s *Server) listFlowers(w http.ResponseWriter, r *http.Request) {
	shopID, err := floristShop(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := catalog.ListFlowers(r.Context(), s.store.Pool, shopID, r.URL.Query().Get("unconfirmed") == "true")
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": nonNilSlice(items)})
}

func (s *Server) listTaxa(w http.ResponseWriter, r *http.Request) {
	items, err := catalog.ListTaxa(r.Context(), s.store.Pool)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": nonNilSlice(items)})
}

type confirmTaxonRequest struct {
	TaxonID int64 `json:"taxon_id" validate:"required,gt=0"`
}

// confirmFlowerTaxon — флорист подтверждает неопознанное растение (FR-C03).
func (s *Server) confirmFlowerTaxon(w http.ResponseWriter, r *http.Request) {
	shopID, err := floristShop(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req confirmTaxonRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	err = s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		if err := catalog.ConfirmTaxon(r.Context(), tx, shopID, id, req.TaxonID); err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, principal(r).UserID, audit.FlowerTaxonConfirmed, "flower", id, map[string]any{"taxon_id": req.TaxonID})
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// updateAvailability — PUT /availability/{id}: наличие, цена и скидка своего магазина.
func (s *Server) updateAvailability(w http.ResponseWriter, r *http.Request) {
	shopID, err := floristShop(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req catalog.AvailabilityUpdate
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if d := req.Validate(); len(d) > 0 {
		writeError(w, r, errValidation(d))
		return
	}
	err = s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		if err := catalog.UpdateAvailability(r.Context(), tx, shopID, id, req); err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, principal(r).UserID, audit.AvailabilityUpdated, "availability", id, map[string]any{
			"in_stock": req.InStock, "price": req.Price, "discount_price": req.DiscountPrice, "discount_until": req.DiscountUntil,
		})
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type shopDTO struct {
	catalog.Shop
	OpenNow bool `json:"open_now"`
}

// listShops — GET /shops: карта, часы, open_now (FR-L02).
func (s *Server) listShops(w http.ResponseWriter, r *http.Request) {
	shops, err := catalog.ListShops(r.Context(), s.store.Pool)
	if err != nil {
		writeError(w, r, err)
		return
	}
	now := s.now()
	out := make([]shopDTO, 0, len(shops))
	for _, sh := range shops {
		out = append(out, shopDTO{Shop: sh, OpenNow: sh.OpeningHours.OpenAt(now, sh.Location())})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
