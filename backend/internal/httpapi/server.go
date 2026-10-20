// Package httpapi — HTTP-слой: роутинг, хендлеры, DTO и маппинг ошибок (§14).
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"github.com/zbekxzz/gulder/backend/internal/assistant"
	"github.com/zbekxzz/gulder/backend/internal/auth"
	"github.com/zbekxzz/gulder/backend/internal/config"
	"github.com/zbekxzz/gulder/backend/internal/ranking"
	"github.com/zbekxzz/gulder/backend/internal/store"
)

// Server собирает зависимости хендлеров.
type Server struct {
	cfg       config.Config
	store     *store.Store
	issuer    *auth.Issuer
	ranking   ranking.Config
	generator assistant.Generator // nil — детерминированный fallback (§9.4)
	now       func() time.Time
}

// Option настраивает Server (для тестов).
type Option func(*Server)

// WithRanking задаёт веса ranker'а (по умолчанию — встроенный ranking.yaml).
func WithRanking(cfg ranking.Config) Option { return func(s *Server) { s.ranking = cfg } }

// WithGenerator задаёт генератор наборов (LLM или мок в тестах).
func WithGenerator(g assistant.Generator) Option { return func(s *Server) { s.generator = g } }

// WithClock подменяет часы.
func WithClock(now func() time.Time) Option { return func(s *Server) { s.now = now } }

// New создаёт сервер.
func New(cfg config.Config, st *store.Store, opts ...Option) *Server {
	s := &Server{
		cfg: cfg, store: st, now: time.Now, ranking: ranking.DefaultConfig(),
		issuer: auth.NewIssuer(cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL),
	}
	// Ключ LLM есть — ассистент на Claude; нет — детерминированный fallback (локально, в тестах и CI).
	if cfg.AnthropicAPIKey != "" {
		s.generator = assistant.NewClaude(cfg.AnthropicAPIKey, cfg.LLMModel, cfg.LLMTimeout)
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Router возвращает корневой http.Handler.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(requestIDMiddleware, accessLog, recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/healthz", s.healthz)
	r.Get("/readyz", s.readyz)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/healthz", s.healthz)
		r.Get("/shops", s.listShops)

		r.Post("/auth/register", s.register)
		r.Post("/auth/login", s.login)
		r.Post("/auth/refresh", s.refresh)

		r.Group(func(r chi.Router) {
			r.Use(s.authenticate)
			r.Get("/me", s.me)
			r.Delete("/me", s.deleteAccount)

			// Даритель (FR-P01–P03, FR-O01)
			r.Group(func(r chi.Router) {
				r.Use(requireRole(auth.RoleGiver))
				r.Post("/recipients", s.createRecipient)
				r.Get("/recipients", s.listRecipients)
				r.Get("/recipients/{id}", s.getRecipient)
				r.Put("/recipients/{id}", s.updateRecipient)
				r.Delete("/recipients/{id}", s.deleteRecipient)
				r.Get("/recipients/{id}/occasions", s.listRecipientOccasions)
				r.Post("/occasions", s.createOccasion)
				r.Get("/occasions/{id}", s.getOccasion)
				r.Post("/recommendations", s.createRecommendations)
				r.Get("/recommendations/{id}", s.getRecommendations)
				r.Get("/recipients/{id}/recommendations", s.listRecipientRecommendations)
				r.Post("/gift-sets", s.createGiftSets)
				r.Post("/gift-sets/{id}/exclude", s.excludeGiftSetItem)
				r.Get("/offers", s.getOffers)
				r.Post("/recommendations/{id}/feedback", s.createFeedback)

				// Исследовательский режим (RM-01, RM-02): по умолчанию выключен
				r.Get("/research/consent", s.getConsent)
				r.Post("/research/consent", s.grantConsent)
				r.Delete("/research/consent", s.revokeConsent)
			})

			// Флорист (FR-C01–C05)
			r.Group(func(r chi.Router) {
				r.Use(requireRole(auth.RoleFlorist))
				r.Post("/catalog/import", s.importCatalog)
				r.Get("/catalog/bouquets", s.listShopBouquets)
				r.Get("/catalog/gifts", s.listShopGifts)
				r.Get("/catalog/flowers", s.listFlowers)
				r.Put("/catalog/flowers/{id}/taxon", s.confirmFlowerTaxon)
				r.Put("/availability/{id}", s.updateAvailability)
			})

			r.Group(func(r chi.Router) {
				r.Use(requireRole(auth.RoleFlorist, auth.RoleManager))
				r.Get("/catalog/taxa", s.listTaxa)
			})

			// FR-S01: замены проверяют и флорист, и даритель
			r.With(requireRole(auth.RoleFlorist, auth.RoleGiver)).Post("/bouquets/{id}/substitutions/validate", s.validateSubstitution)

			// Менеджер (FR-N03)
			r.Group(func(r chi.Router) {
				r.Use(requireRole(auth.RoleManager))
				r.Get("/admin/numerology", s.getNumerology)
				r.Put("/admin/numerology", s.putNumerology)
				r.Get("/admin/data-quality", s.dataQuality)
				r.Get("/admin/analytics", s.analytics)
				r.Get("/admin/rules", s.rulesInfo)
				r.Get("/admin/expert-judgements", s.listJudgements)
				r.Post("/admin/expert-judgements", s.saveJudgement)
				r.Get("/research/evaluation", s.listEvaluations)
				r.Get("/research/evaluation/{run_id}", s.getEvaluation)
				r.Post("/research/export", s.exportResearch)
				r.Get("/research/data-dictionary", s.dataDictionary)
			})
		})

		// P1 (релиз 2.0): контракт заложен, реализация после M8 (§6).
		r.Post("/recipients/{id}/invites", s.notImplemented)
		r.Post("/public/invites/{token}/answers", s.notImplemented)
		r.Post("/orders", s.notImplemented)
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) { writeError(w, r, errNotFound) })
	return r
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyz: БД доступна и миграции применены (§18.4).
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unavailable"})
		return
	}
	ok, err := s.store.MigrationsCurrent(ctx)
	if err != nil || !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "migrations_pending"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) notImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Robots-Tag", "noindex")
	writeError(w, r, errNotImplemented)
}
