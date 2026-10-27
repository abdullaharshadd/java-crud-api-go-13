package main

import (
	"database/sql"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/go-sql-driver/mysql"
	"github.com/rs/zerolog/log"

	"migrated-app/internal/config"
	"migrated-app/internal/httpapi"
	"migrated-app/internal/repository"
	"migrated-app/internal/service"
)

// buildRouter constructs the chi router and mounts every real route of the
// application: the User CRUD endpoints from internal/httpapi.
func buildRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	dsn := ""
	if cfg, err := config.Load(); err != nil {
		log.Error().Err(err).Msg("failed to load config")
	} else {
		dsn = cfg.DatabaseURL
	}
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open database")
	}

	repo := repository.NewMySQLUserRepository(db)
	svc := service.NewUserService(repo)
	h := httpapi.NewHandler(svc, log.Logger)

	// Mount all User routes (POST /save_user_data, GET /get_user_data,
	// GET /get_user_data/{id}, DELETE /delete_user_data/{id},
	// PUT /update_user_data/{id}, GET /get_user_name/name/{name}).
	r.Mount("/", h.Routes())

	return r
}