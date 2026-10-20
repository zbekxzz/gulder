package ranking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/store"
)

// NumerologyVersion — версия таблицы нумерологии (FR-N03).
type NumerologyVersion struct {
	ID        int64             `json:"id"`
	Version   int               `json:"version"`
	IsActive  bool              `json:"is_active"`
	CreatedBy *int64            `json:"created_by"`
	CreatedAt time.Time         `json:"created_at"`
	Rows      []NumerologyEntry `json:"rows,omitempty"`
}

// ActiveNumerology загружает активную версию; нет версий → nil.
func ActiveNumerology(ctx context.Context, q store.Querier) (*NumerologyVersion, error) {
	var v NumerologyVersion
	err := q.QueryRow(ctx, `SELECT id, version, is_active, created_by, created_at FROM numerology_versions WHERE is_active`).
		Scan(&v.ID, &v.Version, &v.IsActive, &v.CreatedBy, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load active numerology: %w", err)
	}
	if v.Rows, err = numerologyRows(ctx, q, v.ID); err != nil {
		return nil, err
	}
	return &v, nil
}

func numerologyRows(ctx context.Context, q store.Querier, versionID int64) ([]NumerologyEntry, error) {
	rows, err := q.Query(ctx, `SELECT number, flowers, colors, meaning FROM numerology_table WHERE version_id = $1 ORDER BY number`, versionID)
	if err != nil {
		return nil, fmt.Errorf("load numerology rows: %w", err)
	}
	defer rows.Close()
	var out []NumerologyEntry
	for rows.Next() {
		e := NumerologyEntry{VersionID: versionID}
		if err := rows.Scan(&e.Number, &e.Flowers, &e.Colors, &e.Meaning); err != nil {
			return nil, fmt.Errorf("scan numerology row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Entry возвращает строку таблицы для числа.
func (v *NumerologyVersion) Entry(number int) *NumerologyEntry {
	if v == nil {
		return nil
	}
	for i := range v.Rows {
		if v.Rows[i].Number == number {
			return &v.Rows[i]
		}
	}
	return nil
}

// ListNumerologyVersions — история версий (без строк).
func ListNumerologyVersions(ctx context.Context, q store.Querier) ([]NumerologyVersion, error) {
	rows, err := q.Query(ctx, `SELECT id, version, is_active, created_by, created_at FROM numerology_versions ORDER BY version DESC`)
	if err != nil {
		return nil, fmt.Errorf("list numerology versions: %w", err)
	}
	defer rows.Close()
	var out []NumerologyVersion
	for rows.Next() {
		var v NumerologyVersion
		if err := rows.Scan(&v.ID, &v.Version, &v.IsActive, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan numerology version: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CreateNumerologyVersion создаёт новую версию таблицы и делает её активной (PUT /admin/numerology).
func CreateNumerologyVersion(ctx context.Context, q store.Querier, managerID int64, rows []NumerologyEntry) (NumerologyVersion, error) {
	var v NumerologyVersion
	if _, err := q.Exec(ctx, `UPDATE numerology_versions SET is_active = false WHERE is_active`); err != nil {
		return v, fmt.Errorf("deactivate numerology: %w", err)
	}
	err := q.QueryRow(ctx, `INSERT INTO numerology_versions (version, created_by, is_active)
		SELECT coalesce(max(version), 0) + 1, $1, true FROM numerology_versions
		RETURNING id, version, is_active, created_by, created_at`, managerID).
		Scan(&v.ID, &v.Version, &v.IsActive, &v.CreatedBy, &v.CreatedAt)
	if err != nil {
		return v, fmt.Errorf("create numerology version: %w", err)
	}
	for _, r := range rows {
		if _, err := q.Exec(ctx, `INSERT INTO numerology_table (version_id, number, flowers, colors, meaning) VALUES ($1,$2,$3,$4,$5)`,
			v.ID, r.Number, nonNil(r.Flowers), nonNil(r.Colors), r.Meaning); err != nil {
			return v, fmt.Errorf("insert numerology row %d: %w", r.Number, err)
		}
		r.VersionID = v.ID
		v.Rows = append(v.Rows, r)
	}
	return v, nil
}
