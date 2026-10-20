package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/audit"
	"github.com/zbekxzz/gulder/backend/internal/auth"
	"github.com/zbekxzz/gulder/backend/internal/catalog"
	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/recipients"
	"github.com/zbekxzz/gulder/backend/internal/recommend"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

type substitutionRequest struct {
	FromFlowerID int64  `json:"from_flower_id" validate:"required,gt=0"`
	ToFlowerID   int64  `json:"to_flower_id" validate:"required,gt=0"`
	OccasionID   *int64 `json:"occasion_id" validate:"omitempty,gt=0"`
	// DryRun — мгновенная проверка в UI без записи в журнал замен.
	DryRun bool `json:"dry_run"`
}

type compositionDTO struct {
	FlowerID  int64  `json:"flower_id"`
	TradeName string `json:"trade_name"`
	Stems     int    `json:"stems"`
}

// validateSubstitution — POST /bouquets/{id}/substitutions/validate (FR-S01, AC-3).
// Любая замена заново прогоняет CheckBouquet; BLOCK → 409 и audit substitution.blocked.
// Ответ — только GiverView: ни даритель, ни флорист не видят скрытых ограничений получателя.
func (s *Server) validateSubstitution(w http.ResponseWriter, r *http.Request) {
	bouquetID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req substitutionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	p := principal(r)
	ctx := r.Context()
	now := s.now()

	snap, err := catalog.LoadSnapshot(ctx, s.store.Pool, now)
	if err != nil {
		writeError(w, r, err)
		return
	}
	b, ok := snap.BouquetByID(bouquetID)
	if !ok {
		writeError(w, r, errNotFound)
		return
	}
	// флорист меняет только букеты своего магазина (§13)
	if p.Role == auth.RoleFlorist && (p.ShopID == nil || *p.ShopID != b.ShopID) {
		writeError(w, r, errNotFound)
		return
	}
	taxa, err := catalog.LoadTaxa(ctx, s.store.Pool)
	if err != nil {
		writeError(w, r, err)
		return
	}
	flowers, err := catalog.LoadFlowers(ctx, s.store.Pool, taxa)
	if err != nil {
		writeError(w, r, err)
		return
	}
	to, ok := flowers[req.ToFlowerID]
	if !ok {
		writeError(w, r, errValidation(map[string]any{"to_flower_id": "цветок не найден"}))
		return
	}

	profile := domain.Profile{} // без повода — проверка «для неизвестного получателя»
	if req.OccasionID != nil {
		var occ recipients.Occasion
		var giverID int64
		if p.Role == auth.RoleGiver {
			occ, err = recipients.GetOccasion(ctx, s.store.Pool, p.UserID, *req.OccasionID)
			giverID = p.UserID
		} else {
			occ, giverID, err = recipients.GetOccasionAny(ctx, s.store.Pool, *req.OccasionID)
		}
		if err != nil {
			writeError(w, r, err)
			return
		}
		rec, err := recipients.Get(ctx, s.store.Pool, giverID, occ.RecipientID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		profile = rec.Profile()
	}

	nb, res, err := rules.ValidateSubstitution(b, req.FromFlowerID, to, profile)
	if errors.Is(err, rules.ErrFlowerNotInBouquet) {
		writeError(w, r, errValidation(map[string]any{"from_flower_id": "этого цветка нет в букете"}))
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	gv := res.GiverView()
	composition := make([]compositionDTO, 0, len(nb.Items))
	for _, it := range nb.Items {
		composition = append(composition, compositionDTO{FlowerID: it.Flower.ID, TradeName: it.Flower.TradeName, Stems: it.Stems})
	}

	if !req.DryRun {
		status, action := "approved", audit.SubstitutionApproved
		if res.Blocked() {
			status, action = "blocked", audit.SubstitutionBlocked
		}
		err = s.store.InTx(ctx, func(tx pgx.Tx) error {
			var subID int64
			// в журнал — только коды нарушений, без сообщений с данными получателя
			if err := tx.QueryRow(ctx, `INSERT INTO substitutions (bouquet_id, from_flower_id, to_flower_id, occasion_id, proposed_by, status, rule_violations)
				VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, bouquetID, req.FromFlowerID, req.ToFlowerID, req.OccasionID, p.UserID, status, res.Codes()).Scan(&subID); err != nil {
				return err
			}
			return audit.Record(ctx, tx, p.UserID, action, "substitution", subID, map[string]any{
				"bouquet_id": bouquetID, "from_flower_id": req.FromFlowerID, "to_flower_id": req.ToFlowerID, "codes": res.Codes(),
			})
		})
		if err != nil {
			writeError(w, r, err)
			return
		}
	}

	if res.Blocked() {
		msgs := make([]string, 0, len(gv.Reasons))
		for _, reason := range gv.Reasons {
			msgs = append(msgs, reason.Message)
		}
		writeError(w, r, errConflict("substitution_blocked", "Замена запрещена: "+strings.Join(msgs, "; "),
			map[string]any{"decision": res.Decision(), "giver_view": gv}))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"decision": res.Decision(), "giver_view": gv, "composition": composition})
}

type feedbackRequest struct {
	BouquetID *int64 `json:"bouquet_id" validate:"omitempty,gt=0"`
	Rating    *int   `json:"rating" validate:"omitempty,min=1,max=5"`
	Liked     *bool  `json:"liked"`
	Comment   string `json:"comment" validate:"max=2000"`
}

// createFeedback — POST /recommendations/{id}/feedback: меняет профиль только получателя этого запуска (FR-F01, AC-8).
func (s *Server) createFeedback(w http.ResponseWriter, r *http.Request) {
	runID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req feedbackRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	if req.Rating == nil && req.Liked == nil && strings.TrimSpace(req.Comment) == "" {
		writeError(w, r, errValidation(map[string]any{"rating": "нужна оценка, реакция или комментарий"}))
		return
	}
	ctx := r.Context()
	giver := principal(r).UserID
	_, recipientID, err := recommend.RunOwner(ctx, s.store.Pool, giver, runID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var sig recipients.FeedbackSignal
	if req.BouquetID != nil {
		snap, err := catalog.LoadSnapshot(ctx, s.store.Pool, s.now())
		if err != nil {
			writeError(w, r, err)
			return
		}
		b, ok := snap.BouquetByID(*req.BouquetID)
		if !ok {
			writeError(w, r, errValidation(map[string]any{"bouquet_id": "букет не найден"}))
			return
		}
		for _, it := range b.Items {
			if it.Flower.Taxon != nil {
				g := strings.ToLower(it.Flower.Taxon.Genus)
				if !contains(sig.Genera, g) {
					sig.Genera = append(sig.Genera, g)
				}
			}
			if c := strings.ToLower(it.Flower.Color); !contains(sig.Colors, c) {
				sig.Colors = append(sig.Colors, c)
			}
		}
	}
	err = s.store.InTx(ctx, func(tx pgx.Tx) error {
		return recipients.SaveFeedback(ctx, tx, recipients.Feedback{
			RunID: runID, RecipientID: recipientID, BouquetID: req.BouquetID, Rating: req.Rating, Liked: req.Liked, Comment: strings.TrimSpace(req.Comment),
		}, sig)
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
