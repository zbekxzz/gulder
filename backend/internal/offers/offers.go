// Package offers — фильтры «Рядом», «Дешевле», «Со скидкой» (§10, FR-L01–L05).
//
// Базовое множество — всегда результат Rules Engine для текущего получателя (FR-L05): фильтры
// работают только поверх допущенных кандидатов и не могут вернуть исключённое. Координаты
// дарителя приходят только из запроса и нигде не сохраняются (FR-L01).
package offers

import (
	"errors"
	"math"
	"slices"
	"strings"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/ranking"
	"github.com/zbekxzz/gulder/backend/internal/recommend"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

// Фильтры.
const (
	FilterNearby   = "nearby"
	FilterCheaper  = "cheaper"
	FilterDiscount = "discount"
)

// Point — координаты дарителя (только в памяти запроса).
type Point struct{ Lat, Lon float64 }

const earthRadiusM = 6371000.0

// HaversineMeters — расстояние по дуге большого круга.
func HaversineMeters(a, b Point) float64 {
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat, dLon := rad(b.Lat-a.Lat), rad(b.Lon-a.Lon)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(a.Lat))*math.Cos(rad(b.Lat))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusM * math.Asin(math.Sqrt(h))
}

// Request — параметры GET /offers.
type Request struct {
	Filters   []string
	Point     *Point
	BouquetID int64 // текущий вариант для «Дешевле»
}

// ParseFilters разбирает список "nearby,discount".
func ParseFilters(s string) ([]string, error) {
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if f != FilterNearby && f != FilterCheaper && f != FilterDiscount {
			return nil, errors.New("допустимые фильтры: nearby, cheaper, discount")
		}
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out, nil
}

// Item — вариант в выдаче фильтров.
type Item struct {
	recommend.Card
	PriceDiff *domain.Money `json:"price_diff,omitempty"` // «Дешевле»: насколько дешевле текущего
}

// GiftOffer — подарок со скидкой (FR-L04).
type GiftOffer struct {
	ID            int64               `json:"id"`
	Name          string              `json:"name"`
	Category      string              `json:"category"`
	ShopID        int64               `json:"shop_id"`
	Price         domain.Money        `json:"price"`
	DiscountPrice *domain.Money       `json:"discount_price"`
	DiscountUntil any                 `json:"discount_until"`
	Warnings      []rules.GiverReason `json:"warnings"`
}

// Response — результат фильтров.
type Response struct {
	Filters []string    `json:"filters"`
	Items   []Item      `json:"items"`
	Gifts   []GiftOffer `json:"gifts"`
	Message string      `json:"message,omitempty"`
}

// Ошибки параметров.
var (
	ErrNeedPoint      = errors.New("для «Рядом» нужны lat и lon")
	ErrNeedBouquet    = errors.New("для «Дешевле» нужен bouquet_id текущего варианта")
	ErrUnknownCurrent = errors.New("текущий вариант не найден среди допущенных")
)

// Apply применяет комбинацию фильтров к допущенным кандидатам (FR-L05).
func Apply(c *recommend.Context, req Request) (Response, error) {
	has := func(f string) bool { return slices.Contains(req.Filters, f) }
	if has(FilterNearby) && req.Point == nil {
		return Response{}, ErrNeedPoint
	}
	if has(FilterCheaper) && req.BouquetID == 0 {
		return Response{}, ErrNeedBouquet
	}
	resp := Response{Filters: nonNil(req.Filters), Items: []Item{}, Gifts: []GiftOffer{}}

	// базовое множество: допущенные Rules Engine и укладывающиеся в бюджет повода
	pool := make([]ranking.Scored, 0, len(c.Scored))
	for _, s := range c.Scored {
		if !s.OverBudget {
			pool = append(pool, s)
		}
	}
	if has(FilterDiscount) {
		pool = slices.DeleteFunc(pool, func(s ranking.Scored) bool { return !s.Bouquet.Offer.DiscountActive(c.Now) })
	}

	var priceDiff *domain.Money
	if has(FilterCheaper) {
		current, ok := c.ScoredByID(req.BouquetID)
		if !ok {
			return Response{}, ErrUnknownCurrent
		}
		// FR-L03: дешевле текущего и максимально близкий по score
		var best *ranking.Scored
		for i, s := range pool {
			if s.Bouquet.ID == current.Bouquet.ID || s.Bouquet.Price >= current.Bouquet.Price {
				continue
			}
			if best == nil || math.Abs(s.Score-current.Score) < math.Abs(best.Score-current.Score) {
				best = &pool[i]
			}
		}
		if best == nil {
			resp.Message = "Дешевле подходящего варианта не нашлось"
			pool = nil
		} else {
			d := current.Bouquet.Price - best.Bouquet.Price
			priceDiff = &d
			pool = []ranking.Scored{*best}
		}
	}

	for i, s := range pool {
		item := Item{Card: c.NewCard(s, i+1), PriceDiff: priceDiff}
		if req.Point != nil {
			if shop, ok := c.Snapshot.Shops[s.Bouquet.ShopID]; ok {
				d := math.Round(HaversineMeters(*req.Point, Point{Lat: shop.Lat, Lon: shop.Lon}))
				item.Shop.DistanceM = &d
			}
		}
		resp.Items = append(resp.Items, item)
	}
	if has(FilterNearby) {
		slices.SortStableFunc(resp.Items, func(a, b Item) int {
			return compareDistance(a.Shop.DistanceM, b.Shop.DistanceM)
		})
		for i := range resp.Items {
			resp.Items[i].Rank = i + 1
		}
	}

	if has(FilterDiscount) {
		for _, g := range c.Snapshot.Gifts {
			if !g.Offer.DiscountActive(c.Now) {
				continue
			}
			res := rules.CheckGift(g, c.Profile)
			if res.Blocked() {
				continue
			}
			resp.Gifts = append(resp.Gifts, GiftOffer{
				ID: g.ID, Name: g.Name, Category: g.Category, ShopID: g.ShopID,
				Price: g.Offer.Price, DiscountPrice: g.Offer.DiscountPrice, DiscountUntil: g.Offer.DiscountUntil, Warnings: res.GiverView().Reasons,
			})
		}
	}
	if len(resp.Items) == 0 && resp.Message == "" {
		resp.Message = "Подходящих вариантов с такими фильтрами нет"
	}
	return resp, nil
}

func compareDistance(a, b *float64) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	case *a < *b:
		return -1
	case *a > *b:
		return 1
	}
	return 0
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
