// Package eval — офлайн-оценка ranker'а (§11): split по получателям, NDCG@5 с bootstrap CI,
// unsafe rate, нарушения бюджета, coverage, ECE и robustness. Нумерология в оценке всегда выключена (FR-N06).
package eval

import (
	"cmp"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"

	"github.com/zbekxzz/gulder/backend/internal/ranking"
)

// NDCGAt — NDCG@k с выигрышем 2^grade − 1. ranked — оценки в порядке выдачи модели,
// all — все оценки случая (для идеальной выдачи). Нет релевантных → 1 (выдача не может быть хуже).
func NDCGAt(k int, ranked, all []int) float64 {
	dcg := func(grades []int) float64 {
		s := 0.0
		for i, g := range grades {
			if i == k {
				break
			}
			s += (math.Pow(2, float64(g)) - 1) / math.Log2(float64(i)+2)
		}
		return s
	}
	ideal := slices.Clone(all)
	slices.SortFunc(ideal, func(a, b int) int { return cmp.Compare(b, a) })
	idcg := dcg(ideal)
	if idcg == 0 {
		return 1
	}
	return dcg(ranked) / idcg
}

// CI — среднее и 95% доверительный интервал.
type CI struct {
	Mean float64    `json:"mean"`
	CI95 [2]float64 `json:"ci95"`
}

// Bootstrap — CI среднего по группам (получателям): группы выбираются с возвращением, iters раз,
// фиксированный seed (§11). groups — значения метрики по получателю (уже усреднённые по его случаям).
func Bootstrap(groups []float64, iters int, seed uint64) CI {
	if len(groups) == 0 {
		return CI{}
	}
	mean := func(xs []float64) float64 {
		s := 0.0
		for _, x := range xs {
			s += x
		}
		return s / float64(len(xs))
	}
	rng := rand.New(rand.NewPCG(seed, 0xb007)) //nolint:gosec // воспроизводимость по seed (§11)
	means := make([]float64, iters)
	sample := make([]float64, len(groups))
	for i := range means {
		for j := range sample {
			sample[j] = groups[rng.IntN(len(groups))]
		}
		means[i] = mean(sample)
	}
	slices.Sort(means)
	q := func(p float64) float64 { return means[min(len(means)-1, int(p*float64(len(means))))] }
	return CI{Mean: mean(groups), CI95: [2]float64{q(0.025), q(0.975)}}
}

// ECE — expected calibration error на bins равных интервалах [0,1].
func ECE(conf []float64, correct []bool, bins int) float64 {
	if len(conf) == 0 {
		return 0
	}
	sumConf := make([]float64, bins)
	sumAcc := make([]float64, bins)
	count := make([]int, bins)
	for i, c := range conf {
		b := min(bins-1, int(c*float64(bins)))
		sumConf[b] += c
		if correct[i] {
			sumAcc[b]++
		}
		count[b]++
	}
	e := 0.0
	for b := 0; b < bins; b++ {
		if count[b] == 0 {
			continue
		}
		n := float64(count[b])
		e += math.Abs(sumAcc[b]/n-sumConf[b]/n) * n / float64(len(conf))
	}
	return e
}

// FitPlatt подбирает sigmoid(a·x + b) методом Ньютона по log loss (§8.1: калибровка на экспертном наборе).
func FitPlatt(x []float64, y []bool) ranking.Platt {
	a, b := 1.0, 0.0
	const l2 = 1e-3 // слабая регуляризация: устойчивость при почти разделимых данных
	for iter := 0; iter < 100; iter++ {
		var ga, gb, haa, hab, hbb float64
		for i, xi := range x {
			p := 1 / (1 + math.Exp(-(a*xi + b)))
			t := 0.0
			if y[i] {
				t = 1
			}
			w := p * (1 - p)
			ga += (p - t) * xi
			gb += p - t
			haa += w * xi * xi
			hab += w * xi
			hbb += w
		}
		ga += l2 * a
		haa += l2
		hbb += l2
		det := haa*hbb - hab*hab
		if math.Abs(det) < 1e-12 {
			break
		}
		da := (hbb*ga - hab*gb) / det
		db := (haa*gb - hab*ga) / det
		a -= da
		b -= db
		if math.Abs(da)+math.Abs(db) < 1e-9 {
			break
		}
	}
	return ranking.Platt{A: a, B: b}
}

// inTest — детерминированное отнесение получателя к test: хэш(seed, ключ) < testFrac.
func inTest(recipientKey string, seed uint64, testFrac float64) bool {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strconv.FormatUint(seed, 10) + "/" + recipientKey))
	return float64(h.Sum64()%10000)/10000 < testFrac
}

// GroupSplit делит случаи по получателям: один получатель не может быть и в train, и в test (§11).
func GroupSplit(cases []Case, seed uint64, testFrac float64) (train, test []Case) {
	for _, c := range cases {
		if inTest(c.RecipientKey, seed, testFrac) {
			test = append(test, c)
		} else {
			train = append(train, c)
		}
	}
	return train, test
}
