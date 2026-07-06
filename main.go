package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	drop "doomsday/drop"
)

func main() {
	_ = godotenv.Load()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// ── Redis ──────────────────────────────────────────────────────────────
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisURL})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}
	logger.Info("redis connected", slog.String("addr", redisURL))

	// ── Postgres ───────────────────────────────────────────────────────────
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:password@localhost:5432/doomsday"
	}
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("postgres pool: %v", err)
	}
	if err := db.Ping(ctx); err != nil {
		log.Fatalf("postgres ping: %v", err)
	}
	logger.Info("postgres connected")

	// ── Services ───────────────────────────────────────────────────────────
	mailer := drop.NewMailer(logger)
	hub := drop.NewHub()
	sched := drop.NewScheduler(db, rdb, hub, mailer, logger)
	sched.Start(ctx)

	h, err := drop.NewHandler(ctx, rdb, db, hub, mailer, logger)
	if err != nil {
		log.Fatalf("handler: %v", err)
	}

	admin := drop.NewAdminHandler(rdb, db, logger)
	otp := drop.NewOTPHandler(db, mailer, logger)

	// ── Routes ─────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Public
	mux.HandleFunc("GET /api/auth/guest", drop.WithCORS(h.GuestToken))
	mux.HandleFunc("POST /api/auth/request-otp", drop.WithCORS(otp.RequestOTP))
	mux.HandleFunc("POST /api/auth/verify-otp", drop.WithCORS(otp.VerifyOTP))
	mux.HandleFunc("GET /api/drops", drop.WithCORS(h.ListDrops))
	mux.HandleFunc("GET /api/drops/{dropID}", drop.WithCORS(h.GetDrop))
	mux.HandleFunc("GET /api/drops/{dropID}/events", hub.ServeSSE)

	// Protected — requires verified user (not guest)
	mux.HandleFunc("POST /api/reserve", drop.UserAuthMiddleware(h.ReserveItem))
	mux.HandleFunc("POST /api/checkout/{reservationID}/complete", drop.UserAuthMiddleware(h.CompleteCheckout))
	mux.HandleFunc("POST /api/waitlist", drop.UserAuthMiddleware(h.JoinWaitlist))

	// Admin
	mux.HandleFunc("POST /api/admin/login", drop.WithCORS(admin.Login))
	mux.HandleFunc("GET /api/admin/stats", drop.AdminAuthMiddleware(admin.Stats))
	mux.HandleFunc("GET /api/admin/drops", drop.AdminAuthMiddleware(admin.ListDrops))
	mux.HandleFunc("POST /api/admin/drops", drop.AdminAuthMiddleware(admin.CreateDrop))
	mux.HandleFunc("PATCH /api/admin/drops/{dropID}/timer", drop.AdminAuthMiddleware(admin.ResetTimer))
	mux.HandleFunc("PATCH /api/admin/drops/{dropID}/stock", drop.AdminAuthMiddleware(admin.ResetStock))
	mux.HandleFunc("GET /api/admin/orders", drop.AdminAuthMiddleware(admin.ListOrders))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	logger.Info("Go API running", slog.String("addr", ":"+port))
	if !mailer.Enabled() {
		logger.Warn("email disabled — set RESEND_API_KEY in .env to enable")
	}
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
