package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	drop "doomsday/drop"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "invariant check failed:", err)
		os.Exit(1)
	}
}

func run() error {
	dropID := flag.String("drop", envOrDefault("LOAD_DROP_ID", drop.DemoDropID), "drop ID to verify")
	repair := flag.Bool("repair", false, "rebuild Redis inventory from PostgreSQL before checking; quiesce writes first")
	timeout := flag.Duration("timeout", 15*time.Second, "overall check timeout")
	flag.Parse()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	redisURL := envOrDefault("REDIS_URL", "localhost:6379")
	redisOptions := &redis.Options{Addr: redisURL}
	if strings.HasPrefix(redisURL, "redis://") || strings.HasPrefix(redisURL, "rediss://") {
		var err error
		redisOptions, err = redis.ParseURL(redisURL)
		if err != nil {
			return errors.New("REDIS_URL is invalid")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer db.Close()
	rdb := redis.NewClient(redisOptions)
	defer rdb.Close()
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("PostgreSQL is unavailable: %w", err)
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis is unavailable: %w", err)
	}

	if *repair {
		if err := drop.ReconcileDropInventory(ctx, db, rdb, *dropID); err != nil {
			return err
		}
	}
	report, err := drop.CheckDropInvariants(ctx, db, rdb, *dropID)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return err
	}
	if !report.Valid {
		return errors.New("stock or order invariants were violated")
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
