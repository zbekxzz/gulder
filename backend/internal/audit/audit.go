// Package audit пишет журнал значимых действий в audit_events (§13).
//
// В payload не кладутся персональные данные получателя (аллергии, питомцы, дата рождения)
// и геолокация — только идентификаторы, коды и счётчики.
package audit

import (
	"context"
	"fmt"

	"github.com/zbekxzz/gulder/backend/internal/store"
)

// Действия audit log.
const (
	CatalogImported       = "catalog.imported"
	AvailabilityUpdated   = "availability.updated"
	FlowerTaxonConfirmed  = "flower.taxon_confirmed"
	SubstitutionApproved  = "substitution.approved"
	SubstitutionBlocked   = "substitution.blocked"
	NumerologyUpdated     = "numerology.updated"
	ConsentGranted        = "research.consent_granted"
	ConsentRevoked        = "research.consent_revoked"
	ResearchExported      = "research.exported"
	RecipientDeleted      = "recipient.deleted"
	AccountDeleted        = "account.deleted"
	ExpertJudgementStored = "expert_judgement.created"
)

// Record добавляет событие. actorID == 0 — системное действие.
func Record(ctx context.Context, q store.Querier, actorID int64, action, entity string, entityID int64, payload map[string]any) error {
	var actor, eid *int64
	if actorID != 0 {
		actor = &actorID
	}
	if entityID != 0 {
		eid = &entityID
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if _, err := q.Exec(ctx, `INSERT INTO audit_events (actor_id, action, entity, entity_id, payload) VALUES ($1,$2,$3,$4,$5)`,
		actor, action, entity, eid, payload); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}
