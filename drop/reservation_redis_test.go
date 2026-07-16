package drop

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestReleaseReservationInRedis(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	const (
		dropID        = "drop-1"
		size          = "M"
		userID        = "user-1"
		reservationID = "reservation-1"
	)

	if err := client.Set(ctx, "drop:drop-1:size:M:stock", 0, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.Set(ctx, "drop:drop-1:stock", 0, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.HSet(ctx, "drop:drop-1:reservations", userID, reservationID).Err(); err != nil {
		t.Fatal(err)
	}

	first, err := releaseReservationInRedis(ctx, client, dropID, size, userID, reservationID)
	if err != nil {
		t.Fatalf("first release failed: %v", err)
	}
	if !first.Released || first.TotalStock != 1 {
		t.Fatalf("first release = %+v, want released with total stock 1", first)
	}
	if stock := client.Get(ctx, "drop:drop-1:size:M:stock").Val(); stock != "1" {
		t.Fatalf("size stock = %s, want 1", stock)
	}
	if client.HExists(ctx, "drop:drop-1:reservations", userID).Val() {
		t.Fatal("reservation marker still exists after release")
	}

	second, err := releaseReservationInRedis(ctx, client, dropID, size, userID, reservationID)
	if err != nil {
		t.Fatalf("second release failed: %v", err)
	}
	if second.Released || second.TotalStock != 1 {
		t.Fatalf("second release = %+v, want idempotent no-op with total stock 1", second)
	}
	if stock := client.Get(ctx, "drop:drop-1:size:M:stock").Val(); stock != "1" {
		t.Fatalf("size stock after retry = %s, want 1", stock)
	}
}

func TestReleaseReservationDoesNotTouchNewerMarker(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	client.Set(ctx, "drop:drop-1:size:L:stock", 3, 0)
	client.Set(ctx, "drop:drop-1:stock", 8, 0)
	client.HSet(ctx, "drop:drop-1:reservations", "user-1", "new-reservation")

	result, err := releaseReservationInRedis(ctx, client, "drop-1", "L", "user-1", "old-reservation")
	if err != nil {
		t.Fatal(err)
	}
	if result.Released || result.TotalStock != 8 {
		t.Fatalf("release = %+v, want no-op with total stock 8", result)
	}
	if marker := client.HGet(ctx, "drop:drop-1:reservations", "user-1").Val(); marker != "new-reservation" {
		t.Fatalf("marker = %q, want newer reservation", marker)
	}
}
