package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
)

// Виды импорта флориста (POST /catalog/import?kind=…).
const (
	KindFlowers  = "flowers"
	KindBouquets = "bouquets"
	KindGifts    = "gifts"
)

// ImportReport — ответ импорта: {imported, updated, errors:[{line, field, message}]} (§12).
type ImportReport struct {
	Imported int        `json:"imported"`
	Updated  int        `json:"updated"`
	Errors   []RowError `json:"errors"`
}

// FormatError — файл не читается как CSV нужного вида (нет колонок и т.п.).
type FormatError struct{ Err error }

func (e *FormatError) Error() string { return e.Err.Error() }

func (e *FormatError) Unwrap() error { return e.Err }

// ErrUnknownKind — неизвестный вид импорта.
var ErrUnknownKind = errors.New("unknown import kind")

// Import загружает CSV флориста в его магазин в одной транзакции (§18.2). Строки с ошибками
// пропускаются и попадают в отчёт; остальные применяются (каждая — в своей точке сохранения).
func Import(ctx context.Context, tx pgx.Tx, shopID int64, kind string, r io.Reader) (ImportReport, error) {
	rep := ImportReport{Errors: []RowError{}}
	switch kind {
	case KindFlowers:
		rows, errs, err := ParseFlowers(r)
		if err != nil {
			return rep, &FormatError{Err: err}
		}
		rep.Errors = append(rep.Errors, errs...)
		bad := badLines(errs)
		for _, row := range rows {
			if bad[row.Line] {
				continue
			}
			applyRow(ctx, tx, &rep, row.Line, func(sp pgx.Tx) (bool, error) { return upsertShopFlower(ctx, sp, shopID, row) })
		}
	case KindBouquets:
		rows, errs, err := ParseBouquets(r)
		if err != nil {
			return rep, &FormatError{Err: err}
		}
		rep.Errors = append(rep.Errors, errs...)
		bad := badLines(errs)
		for _, row := range rows {
			if bad[row.Line] {
				continue
			}
			if len(row.Composition) == 0 {
				rep.Errors = append(rep.Errors, RowError{Line: row.Line, Field: "composition", Message: "обязательное поле"})
				continue
			}
			applyRow(ctx, tx, &rep, row.Line, func(sp pgx.Tx) (bool, error) { return InsertBouquet(ctx, sp, shopID, row, true) })
		}
	case KindGifts:
		rows, errs, err := ParseGifts(r)
		if err != nil {
			return rep, &FormatError{Err: err}
		}
		rep.Errors = append(rep.Errors, errs...)
		bad := badLines(errs)
		for _, row := range rows {
			if bad[row.Line] {
				continue
			}
			applyRow(ctx, tx, &rep, row.Line, func(sp pgx.Tx) (bool, error) { return InsertGift(ctx, sp, shopID, row, true) })
		}
	default:
		return rep, ErrUnknownKind
	}
	return rep, nil
}

func badLines(errs []RowError) map[int]bool {
	m := map[int]bool{}
	for _, e := range errs {
		m[e.Line] = true
	}
	return m
}

// applyRow выполняет запись строки в точке сохранения; ошибка откатывает только эту строку.
func applyRow(ctx context.Context, tx pgx.Tx, rep *ImportReport, line int, fn func(pgx.Tx) (bool, error)) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		rep.Errors = append(rep.Errors, RowError{Line: line, Message: "внутренняя ошибка"})
		return
	}
	created, err := fn(sp)
	if err != nil {
		_ = sp.Rollback(ctx)
		rep.Errors = append(rep.Errors, RowError{Line: line, Message: rowMessage(err)})
		return
	}
	if err := sp.Commit(ctx); err != nil {
		rep.Errors = append(rep.Errors, RowError{Line: line, Message: "внутренняя ошибка"})
		return
	}
	if created {
		rep.Imported++
	} else {
		rep.Updated++
	}
}

// ImportError — ошибка строки, понятная флористу.
type ImportError struct{ Msg string }

func (e *ImportError) Error() string { return e.Msg }

func rowMessage(err error) string {
	var ie *ImportError
	if errors.As(err, &ie) {
		return ie.Msg
	}
	return "не удалось сохранить строку"
}

// upsertShopFlower создаёт или обновляет цветок магазина. Неизвестный род → taxon_id NULL,
// taxon_confirmed=false: до подтверждения флористом Rules Engine блокирует такие букеты (FR-C03).
func upsertShopFlower(ctx context.Context, tx pgx.Tx, shopID int64, r FlowerRow) (bool, error) {
	var owner *int64
	err := tx.QueryRow(ctx, `SELECT shop_id FROM flowers WHERE code = $1`, r.Code).Scan(&owner)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("check flower: %w", err)
	}
	if exists && (owner == nil || *owner != shopID) {
		return false, &ImportError{Msg: fmt.Sprintf("код %q занят общим справочником или другим магазином", r.Code)}
	}
	taxonID, err := ResolveTaxon(ctx, tx, r.ScientificName, r.Genus)
	if err != nil {
		return false, err
	}
	confirmed := r.TaxonConfirmed && taxonID != nil
	if exists {
		_, err = tx.Exec(ctx, `UPDATE flowers SET taxon_id=$2, taxon_confirmed=$3, trade_name=$4, color=$5, scent=$6,
			season_months=$7, care_notes=NULLIF($8,''), restrictions=NULLIF($9,'') WHERE code=$1`,
			r.Code, taxonID, confirmed, r.TradeName, r.Color, string(r.Scent), r.SeasonMonths, r.CareNotes, r.Restrictions)
		if err != nil {
			return false, fmt.Errorf("update flower: %w", err)
		}
		return false, nil
	}
	_, err = tx.Exec(ctx, `INSERT INTO flowers (code, shop_id, taxon_id, taxon_confirmed, trade_name, color, scent, season_months, care_notes, restrictions)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''))`,
		r.Code, shopID, taxonID, confirmed, r.TradeName, r.Color, string(r.Scent), r.SeasonMonths, r.CareNotes, r.Restrictions)
	if err != nil {
		return false, fmt.Errorf("insert flower: %w", err)
	}
	return true, nil
}
