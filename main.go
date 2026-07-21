package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	drop "doomsday/drop"
)

func main() {
	_ = godotenv.Load()
	if err := drop.ValidateSecurityConfig(); err != nil {
		log.Fatalf("security configuration: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := drop.NewStructuredLogger(os.Stdout)
	telemetry, shutdownTelemetry, err := drop.NewTelemetry(ctx, logger)
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(shutdownCtx); err != nil {
			logger.Error("telemetry shutdown failed", slog.Any("err", err))
		}
	}()

	// ── Redis ──────────────────────────────────────────────────────────────
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}
	redisOptions := &redis.Options{Addr: redisURL}
	if strings.HasPrefix(redisURL, "redis://") || strings.HasPrefix(redisURL, "rediss://") {
		var err error
		redisOptions, err = redis.ParseURL(redisURL)
		if err != nil {
			log.Fatalf("redis URL: %v", err)
		}
	}
	rdb := redis.NewClient(redisOptions)
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}
	logger.Info("redis connected", slog.String("addr", redisOptions.Addr))

	// ── Postgres ───────────────────────────────────────────────────────────
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("postgres pool: %v", err)
	}
	if err := db.Ping(ctx); err != nil {
		log.Fatalf("postgres ping: %v", err)
	}
	defer db.Close()
	logger.Info("postgres connected")

	if strings.EqualFold(os.Getenv("DEMO_RESET_ON_START"), "true") {
		if err := drop.ResetDemoData(ctx, db, rdb); err != nil {
			log.Fatalf("demo reset: %v", err)
		}
		logger.Info("demo data reset on startup", slog.String("drop_id", drop.DemoDropID))
	}

	// ── Services ───────────────────────────────────────────────────────────
	mailer := drop.NewMailer(logger)
	hub := drop.NewHub()

	h, err := drop.NewHandler(ctx, rdb, db, hub, mailer, logger, os.Getenv("PAYMENT_WEBHOOK_SECRET"))
	if err != nil {
		log.Fatalf("handler: %v", err)
	}
	h.SetTelemetry(telemetry)
	dispatcher := drop.NewApplicationJobDispatcher(db, rdb, hub, mailer, logger, h)
	worker := drop.NewOutboxWorker(db, dispatcher, logger, telemetry)
	worker.Start(ctx)
	sched := drop.NewScheduler(db, logger)
	sched.Start(ctx)

	admin := drop.NewAdminHandler(rdb, db, logger)
	otp := drop.NewOTPHandler(db, rdb, mailer, logger)

	// ── Routes ─────────────────────────────────────────────────────────────
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		checkCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(checkCtx); err != nil {
			http.Error(w, `{"status":"unavailable","dependency":"postgres"}`, http.StatusServiceUnavailable)
			return
		}
		if err := rdb.Ping(checkCtx).Err(); err != nil {
			http.Error(w, `{"status":"unavailable","dependency":"redis"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	mux.Handle("GET /health/dependencies", drop.NewDependencyHealthHandler(
		func(ctx context.Context) error { return db.Ping(ctx) },
		func(ctx context.Context) error { return rdb.Ping(ctx).Err() },
	))
	mux.Handle("GET /metrics", telemetry.Handler())

	// Public
	mux.HandleFunc("GET /api/auth/guest", drop.WithCORS(h.GuestToken))
	mux.HandleFunc("POST /api/auth/request-otp", drop.WithCORS(otp.RequestOTP))
	mux.HandleFunc("POST /api/auth/verify-otp", drop.WithCORS(otp.VerifyOTP))
	mux.HandleFunc("GET /api/drops", drop.WithCORS(h.ListDrops))
	mux.HandleFunc("GET /api/drops/{dropID}", drop.WithCORS(h.GetDrop))
	mux.HandleFunc("GET /api/drops/{dropID}/events", drop.WithCORS(hub.ServeSSE))

	// Protected — requires verified user (not guest)
	mux.HandleFunc("POST /api/reserve", drop.UserAuthMiddleware(h.ReserveItem))
	mux.HandleFunc("POST /api/checkout/{reservationID}/payments", drop.UserAuthMiddleware(h.CreatePayment))
	mux.HandleFunc("GET /api/payments/{paymentID}", drop.UserAuthMiddleware(h.GetPayment))
	mux.HandleFunc("POST /api/payments/webhook", h.PaymentWebhook)
	mux.HandleFunc("POST /api/waitlist", drop.UserAuthMiddleware(h.JoinWaitlist))

	// Admin
	mux.HandleFunc("POST /api/admin/login", drop.WithCORS(admin.Login))
	mux.HandleFunc("GET /api/admin/stats", drop.AdminAuthMiddleware(admin.Stats))
	mux.HandleFunc("GET /api/admin/drops", drop.AdminAuthMiddleware(admin.ListDrops))
	mux.HandleFunc("POST /api/admin/drops", drop.AdminAuthMiddleware(admin.CreateDrop))
	mux.HandleFunc("PATCH /api/admin/drops/{dropID}/timer", drop.AdminAuthMiddleware(admin.ResetTimer))
	mux.HandleFunc("PATCH /api/admin/drops/{dropID}/stock", drop.AdminAuthMiddleware(admin.ResetStock))
	mux.HandleFunc("GET /api/admin/orders", drop.AdminAuthMiddleware(admin.ListOrders))
	mux.HandleFunc("POST /api/admin/payments/{paymentID}/refund", drop.AdminAuthMiddleware(h.RefundPayment))
	if drop.DemoModeEnabled() {
		mux.HandleFunc("POST /api/admin/demo/reset", drop.AdminAuthMiddleware(admin.ResetDemo))
	}
	if os.Getenv("APP_ENV") == "test" {
		mux.HandleFunc("POST /api/admin/test/reservations/{reservationID}/expire", drop.AdminAuthMiddleware(h.ExpireReservationForTest))
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           drop.ObservabilityMiddleware(logger, telemetry)(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	logger.Info("Go API running", slog.String("addr", server.Addr))
	if !mailer.Enabled() {
		logger.Warn("email disabled — set RESEND_API_KEY in .env to enable")
	}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server: %v", err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP shutdown failed", slog.Any("err", err))
		}
	}
	stop()
	worker.Wait()
}
