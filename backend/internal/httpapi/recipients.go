package httpapi

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/audit"
	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/recipients"
)

// attributeDTO — поле профиля в ответе. Поля, заполненные получателем, скрыты: {"key","hidden":true} (§13).
type attributeDTO struct {
	Key        string             `json:"key"`
	Value      any                `json:"value,omitempty"`
	Source     *domain.Source     `json:"source,omitempty"`
	Confidence *domain.Confidence `json:"confidence,omitempty"`
	UpdatedAt  *time.Time         `json:"updated_at,omitempty"`
	Hidden     bool               `json:"hidden,omitempty"`
}

type recipientDTO struct {
	ID         int64          `json:"id"`
	Nickname   string         `json:"nickname"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Attributes []attributeDTO `json:"attributes,omitempty"`
}

// toRecipientDTO возвращает все ключи профиля: отсутствующие — как confidence=unknown
// без значения (FR-P03: «не знаю» ≠ «нет»), скрытые — только с hidden=true.
func toRecipientDTO(r recipients.Recipient, withAttributes bool) recipientDTO {
	dto := recipientDTO{ID: r.ID, Nickname: r.Nickname, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	if !withAttributes {
		return dto
	}
	stored := map[string]recipients.StoredAttribute{}
	for _, a := range r.Attributes {
		stored[a.Key] = a
	}
	for _, key := range domain.AttributeKeys {
		a, ok := stored[key]
		switch {
		case !ok:
			unknown := domain.Unknown
			dto.Attributes = append(dto.Attributes, attributeDTO{Key: key, Confidence: &unknown})
		case a.Source == domain.SourceRecipient:
			dto.Attributes = append(dto.Attributes, attributeDTO{Key: key, Hidden: true})
		default:
			src, conf, upd := a.Source, a.Confidence, a.UpdatedAt
			var value any
			if a.Confidence != domain.Unknown {
				value = a.Value
			}
			dto.Attributes = append(dto.Attributes, attributeDTO{Key: key, Value: value, Source: &src, Confidence: &conf, UpdatedAt: &upd})
		}
	}
	return dto
}

type createRecipientRequest struct {
	Nickname   string                      `json:"nickname" validate:"required,min=1,max=80"`
	Attributes []recipients.AttributeInput `json:"attributes"`
}

type updateRecipientRequest struct {
	Nickname   *string                     `json:"nickname" validate:"omitempty,min=1,max=80"`
	Attributes []recipients.AttributeInput `json:"attributes"`
}

// validateAttributes нормализует поля, присланные дарителем. Даритель пишет только source=giver:
// ответы получателя приходят через приглашение (P1), feedback — через свой эндпоинт.
func (s *Server) validateAttributes(in []recipients.AttributeInput) ([]recipients.Attribute, error) {
	details := map[string]any{}
	out := make([]recipients.Attribute, 0, len(in))
	seen := map[string]bool{}
	for _, a := range in {
		if a.Source != "" && a.Source != domain.SourceGiver {
			details[a.Key] = "даритель может указывать только source=giver"
			continue
		}
		if seen[a.Key] {
			details[a.Key] = "ключ повторяется"
			continue
		}
		seen[a.Key] = true
		norm, err := recipients.ValidateAttribute(a, s.now())
		if err != nil {
			details[a.Key] = err.Error()
			continue
		}
		out = append(out, norm)
	}
	if len(details) > 0 {
		return nil, errValidation(details)
	}
	return out, nil
}

func (s *Server) createRecipient(w http.ResponseWriter, r *http.Request) {
	var req createRecipientRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	attrs, err := s.validateAttributes(req.Attributes)
	if err != nil {
		writeError(w, r, err)
		return
	}
	giver := principal(r).UserID
	var rec recipients.Recipient
	err = s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		created, err := recipients.Create(r.Context(), tx, giver, req.Nickname)
		if err != nil {
			return err
		}
		if err := recipients.UpsertAttributes(r.Context(), tx, created.ID, attrs, false); err != nil {
			return err
		}
		rec, err = recipients.Get(r.Context(), tx, giver, created.ID)
		return err
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toRecipientDTO(rec, true))
}

func (s *Server) listRecipients(w http.ResponseWriter, r *http.Request) {
	list, err := recipients.List(r.Context(), s.store.Pool, principal(r).UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]recipientDTO, 0, len(list))
	for _, rec := range list {
		out = append(out, toRecipientDTO(rec, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) getRecipient(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	rec, err := recipients.Get(r.Context(), s.store.Pool, principal(r).UserID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toRecipientDTO(rec, true))
}

func (s *Server) updateRecipient(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req updateRecipientRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	attrs, err := s.validateAttributes(req.Attributes)
	if err != nil {
		writeError(w, r, err)
		return
	}
	giver := principal(r).UserID
	var rec recipients.Recipient
	err = s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		if _, err := recipients.Get(r.Context(), tx, giver, id); err != nil {
			return err
		}
		if req.Nickname != nil {
			if err := recipients.Rename(r.Context(), tx, id, *req.Nickname); err != nil {
				return err
			}
		}
		if err := recipients.UpsertAttributes(r.Context(), tx, id, attrs, false); err != nil {
			return err
		}
		rec, err = recipients.Get(r.Context(), tx, giver, id)
		return err
	})
	if errors.Is(err, recipients.ErrRecipientOwned) {
		writeError(w, r, errConflict("recipient_owned", "Поле заполнено получателем и не может быть изменено дарителем", nil))
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toRecipientDTO(rec, true))
}

func (s *Server) deleteRecipient(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	giver := principal(r).UserID
	err = s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		if err := recipients.Delete(r.Context(), tx, giver, id); err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, giver, audit.RecipientDeleted, "recipient", id, nil)
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type occasionRequest struct {
	RecipientID       int64        `json:"recipient_id" validate:"required,gt=0"`
	Type              string       `json:"type" validate:"required"`
	Relationship      string       `json:"relationship"`
	Date              *string      `json:"date" validate:"omitempty,datetime=2006-01-02"`
	BudgetMin         domain.Money `json:"budget_min" validate:"gte=0"`
	BudgetMax         domain.Money `json:"budget_max" validate:"gt=0"`
	Place             string       `json:"place"`
	Formality         *int         `json:"formality" validate:"omitempty,min=1,max=5"`
	EmotionGoal       string       `json:"emotion_goal"`
	NumerologyEnabled bool         `json:"numerology_enabled"`
}

// createOccasion — FR-O01; нумерология по умолчанию выключена (FR-N01).
func (s *Server) createOccasion(w http.ResponseWriter, r *http.Request) {
	var req occasionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	details := map[string]any{}
	if req.BudgetMin > req.BudgetMax {
		details["budget_min"] = "не больше максимального бюджета"
	}
	checkEnum(details, "type", req.Type, recipients.OccasionTypes, false)
	checkEnum(details, "relationship", req.Relationship, recipients.Relationships, true)
	checkEnum(details, "place", req.Place, recipients.Places, true)
	checkEnum(details, "emotion_goal", req.EmotionGoal, recipients.Emotions, true)
	if len(details) > 0 {
		writeError(w, r, errValidation(details))
		return
	}
	o, err := recipients.CreateOccasion(r.Context(), s.store.Pool, principal(r).UserID, recipients.Occasion{
		RecipientID: req.RecipientID, Type: req.Type, Relationship: req.Relationship, Date: req.Date,
		BudgetMin: req.BudgetMin, BudgetMax: req.BudgetMax, Place: req.Place, Formality: req.Formality,
		EmotionGoal: req.EmotionGoal, NumerologyEnabled: req.NumerologyEnabled,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func checkEnum(details map[string]any, field, v string, allowed []string, optional bool) {
	if v == "" && optional {
		return
	}
	if !slices.Contains(allowed, v) {
		details[field] = allowed
	}
}

func (s *Server) getOccasion(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	o, err := recipients.GetOccasion(r.Context(), s.store.Pool, principal(r).UserID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) listRecipientOccasions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	giver := principal(r).UserID
	if _, err := recipients.Get(r.Context(), s.store.Pool, giver, id); err != nil {
		writeError(w, r, err)
		return
	}
	list, err := recipients.ListOccasions(r.Context(), s.store.Pool, giver, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if list == nil {
		list = []recipients.Occasion{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}
