package main

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jmlane8/lending-backend-quick-claude/internal/auth"
	"github.com/jmlane8/lending-backend-quick-claude/internal/config"
	"github.com/jmlane8/lending-backend-quick-claude/internal/loanapplication"
)

// routes returns the feature route registrations.
func routes(cfg config.Config, pool *pgxpool.Pool, reg prometheus.Registerer) []func(*http.ServeMux) {
	verifier := auth.NewVerifier(cfg.JWTPublicKey, cfg.JWTIssuer, cfg.JWTAudience)
	loans := loanapplication.NewService(pool, loanapplication.ServiceConfig{
		Currency:   cfg.Currency,
		Location:   cfg.Location,
		Registerer: reg,
	})
	return []func(*http.ServeMux){
		loanapplication.NewHandler(loans, verifier).Register,
	}
}
