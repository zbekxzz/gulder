package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// OpeningHours — часы работы: {"mon":[["09:00","21:00"]], "sun":[]} (FR-C05).
// Несколько интервалов в день; "24:00" — конец суток. Отсутствующий день = выходной.
type OpeningHours map[string][][2]string

var weekdayKeys = [...]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// ParseOpeningHours разбирает и проверяет JSON часов работы.
func ParseOpeningHours(raw []byte) (OpeningHours, error) {
	var h OpeningHours
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, errors.New(`ожидается JSON вида {"mon":[["09:00","21:00"]]}`)
	}
	for day, intervals := range h {
		if !validDay(day) {
			return nil, fmt.Errorf("неизвестный день %q", day)
		}
		for _, iv := range intervals {
			from, err1 := minutes(iv[0])
			to, err2 := minutes(iv[1])
			if err1 != nil || err2 != nil || from >= to {
				return nil, fmt.Errorf("некорректный интервал %v для %s", iv, day)
			}
		}
	}
	return h, nil
}

func validDay(d string) bool {
	for _, k := range weekdayKeys {
		if k == d {
			return true
		}
	}
	return false
}

func minutes(hhmm string) (int, error) {
	h, m, ok := strings.Cut(hhmm, ":")
	hi, err1 := strconv.Atoi(h)
	mi, err2 := strconv.Atoi(m)
	if !ok || err1 != nil || err2 != nil || hi < 0 || hi > 24 || mi < 0 || mi > 59 || (hi == 24 && mi != 0) {
		return 0, fmt.Errorf("bad time %q", hhmm)
	}
	return hi*60 + mi, nil
}

// OpenAt сообщает, открыт ли магазин в момент t по местному времени loc.
func (h OpeningHours) OpenAt(t time.Time, loc *time.Location) bool {
	local := t.In(loc)
	now := local.Hour()*60 + local.Minute()
	for _, iv := range h[weekdayKeys[local.Weekday()]] {
		from, err1 := minutes(iv[0])
		to, err2 := minutes(iv[1])
		if err1 == nil && err2 == nil && now >= from && now < to {
			return true
		}
	}
	return false
}
