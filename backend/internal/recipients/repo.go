package recipients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/store"
)

// ErrRecipientOwned — поле заполнено получателем, даритель не может его менять (§13).
var ErrRecipientOwned = errors.New("attribute is owned by recipient")

// Recipient — получатель с полями профиля.
type Recipient struct {
	ID         int64
	GiverID    int64
	Nickname   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Attributes []StoredAttribute
}

// StoredAttribute — поле профиля из БД.
type StoredAttribute struct {
	Key        string
	Value      any
	Source     domain.Source
	Confidence domain.Confidence
	Reusable   bool
	UpdatedAt  time.Time
}

// Create создаёт получателя (FR-P01).
func Create(ctx context.Context, q store.Querier, giverID int64, nickname string) (Recipient, error) {
	r := Recipient{GiverID: giverID, Nickname: nickname}
	err := q.QueryRow(ctx, `INSERT INTO recipients (giver_id, nickname) VALUES ($1,$2) RETURNING id, created_at, updated_at`,
		giverID, nickname).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return r, fmt.Errorf("insert recipient: %w", err)
	}
	return r, nil
}

// List возвращает получателей дарителя.
func List(ctx context.Context, q store.Querier, giverID int64) ([]Recipient, error) {
	rows, err := q.Query(ctx, `SELECT id, giver_id, nickname, created_at, updated_at FROM recipients
		WHERE giver_id = $1 ORDER BY created_at DESC, id DESC`, giverID)
	if err != nil {
		return nil, fmt.Errorf("list recipients: %w", err)
	}
	defer rows.Close()
	var out []Recipient
	for rows.Next() {
		var r Recipient
		if err := rows.Scan(&r.ID, &r.GiverID, &r.Nickname, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan recipient: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Get возвращает получателя дарителя с профилем; чужой или несуществующий → store.ErrNotFound.
func Get(ctx context.Context, q store.Querier, giverID, id int64) (Recipient, error) {
	var r Recipient
	err := q.QueryRow(ctx, `SELECT id, giver_id, nickname, created_at, updated_at FROM recipients
		WHERE id = $1 AND giver_id = $2`, id, giverID).Scan(&r.ID, &r.GiverID, &r.Nickname, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, store.ErrNotFound
	}
	if err != nil {
		return r, fmt.Errorf("get recipient: %w", err)
	}
	r.Attributes, err = loadAttributes(ctx, q, id)
	return r, err
}

func loadAttributes(ctx context.Context, q store.Querier, recipientID int64) ([]StoredAttribute, error) {
	rows, err := q.Query(ctx, `SELECT key, value, source, confidence, reusable, updated_at
		FROM recipient_attributes WHERE recipient_id = $1 ORDER BY key`, recipientID)
	if err != nil {
		return nil, fmt.Errorf("load attributes: %w", err)
	}
	defer rows.Close()
	var out []StoredAttribute
	for rows.Next() {
		var a StoredAttribute
		var raw []byte
		if err := rows.Scan(&a.Key, &raw, &a.Source, &a.Confidence, &a.Reusable, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan attribute: %w", err)
		}
		if raw != nil {
			if err := json.Unmarshal(raw, &a.Value); err != nil {
				return nil, fmt.Errorf("decode attribute %s: %w", a.Key, err)
			}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Profile строит профиль для Rules Engine. Отсутствующий ключ = unknown.
func (r Recipient) Profile() domain.Profile {
	p := domain.Profile{}
	for _, a := range r.Attributes {
		p[a.Key] = domain.Attribute{Value: a.Value, Source: a.Source, Confidence: a.Confidence, UpdatedAt: a.UpdatedAt}
	}
	return p
}

// Rename меняет отображаемое имя.
func Rename(ctx context.Context, q store.Querier, id int64, nickname string) error {
	if _, err := q.Exec(ctx, `UPDATE recipients SET nickname = $2, updated_at = now() WHERE id = $1`, id, nickname); err != nil {
		return fmt.Errorf("rename recipient: %w", err)
	}
	return nil
}

// UpsertAttributes записывает поля профиля (FR-P03). allowRecipientOverwrite=false запрещает
// дарителю менять поля, заполненные получателем.
func UpsertAttributes(ctx context.Context, q store.Querier, recipientID int64, attrs []Attribute, allowRecipientOverwrite bool) error {
	for _, a := range attrs {
		if !allowRecipientOverwrite {
			var src domain.Source
			err := q.QueryRow(ctx, `SELECT source FROM recipient_attributes WHERE recipient_id=$1 AND key=$2`, recipientID, a.Key).Scan(&src)
			if err == nil && src == domain.SourceRecipient {
				return fmt.Errorf("%s: %w", a.Key, ErrRecipientOwned)
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("check attribute: %w", err)
			}
		}
		var value any
		if a.Value != nil {
			b, err := json.Marshal(a.Value)
			if err != nil {
				return fmt.Errorf("encode %s: %w", a.Key, err)
			}
			value = b
		}
		if _, err := q.Exec(ctx, `
			INSERT INTO recipient_attributes (recipient_id, key, value, source, confidence)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (recipient_id, key) DO UPDATE
			   SET value = EXCLUDED.value, source = EXCLUDED.source, confidence = EXCLUDED.confidence, updated_at = now()`,
			recipientID, a.Key, value, string(a.Source), string(a.Confidence)); err != nil {
			return fmt.Errorf("upsert attribute %s: %w", a.Key, err)
		}
	}
	if _, err := q.Exec(ctx, `UPDATE recipients SET updated_at = now() WHERE id = $1`, recipientID); err != nil {
		return fmt.Errorf("touch recipient: %w", err)
	}
	return nil
}

// Delete удаляет получателя дарителя; атрибуты (включая дату рождения), поводы и приглашения
// удаляются каскадом (FR-N07, §13).
func Delete(ctx context.Context, q store.Querier, giverID, id int64) error {
	tag, err := q.Exec(ctx, `DELETE FROM recipients WHERE id = $1 AND giver_id = $2`, id, giverID)
	if err != nil {
		return fmt.Errorf("delete recipient: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}
