package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Money — сумма в тиынах (1 тенге = 100 тиынов). Никогда не float (§7.2, §18.3; ADR 0002).
type Money int64

// Tenge создаёт сумму из целых тенге.
func Tenge(t int64) Money { return Money(t * 100) }

// ParseMoney разбирает десятичную запись тенге: "12000", "12000.5", "12000.50".
// Больше двух знаков после точки — ошибка, округления нет.
func ParseMoney(s string) (Money, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty amount")
	}
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, frac, hasFrac := strings.Cut(s, ".")
	if intPart == "" || (hasFrac && (frac == "" || len(frac) > 2)) {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	for _, r := range intPart + frac {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid amount %q", s)
		}
	}
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount %q: %w", s, err)
	}
	for len(frac) < 2 {
		frac += "0"
	}
	cents, _ := strconv.ParseInt(frac, 10, 64)
	v := whole*100 + cents
	if neg {
		v = -v
	}
	return Money(v), nil
}

// String возвращает запись с двумя знаками: "12000.00".
func (m Money) String() string {
	sign := ""
	v := int64(m)
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

// jsonNumber — минимальная десятичная запись: 15000, 129.5, 129.05.
func (m Money) jsonNumber() string {
	s := m.String()
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// MarshalJSON кодирует сумму числом в тенге.
func (m Money) MarshalJSON() ([]byte, error) { return []byte(m.jsonNumber()), nil }

// UnmarshalJSON принимает число или строку в тенге.
func (m *Money) UnmarshalJSON(b []byte) error {
	var s string
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("money: %w", err)
		}
	} else {
		s = string(b)
	}
	v, err := ParseMoney(s)
	if err != nil {
		return err
	}
	*m = v
	return nil
}

// Scan читает numeric(12,2) из БД (pgx отдаёт numeric строкой).
func (m *Money) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return m.scanString(v)
	case []byte:
		return m.scanString(string(v))
	case int64:
		*m = Tenge(v)
		return nil
	}
	return fmt.Errorf("money: cannot scan %T", src)
}

func (m *Money) scanString(s string) error {
	// numeric может прийти как "12000" или "12000.00"
	v, err := ParseMoney(s)
	if err != nil {
		return err
	}
	*m = v
	return nil
}

// Value пишет сумму в БД десятичной строкой.
func (m Money) Value() (driver.Value, error) { return m.String(), nil }
