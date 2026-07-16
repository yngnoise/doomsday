package drop

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestDistributeAvailableStock(t *testing.T) {
	labels := []string{"S", "M", "L"}
	consumed := map[string]int{"S": 1, "M": 2, "L": 3}

	result := distributeAvailableStock(labels, consumed, 8)
	wantAvailable := []int{2, 2, 4}
	wantBaseline := []int{3, 4, 7}
	for index, size := range result {
		if size.Available != wantAvailable[index] || size.Baseline != wantBaseline[index] {
			t.Fatalf("size %s = %+v, want available %d and baseline %d", size.Label, size, wantAvailable[index], wantBaseline[index])
		}
	}
}

func TestReplaceStockInRedis(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	client.Set(ctx, "drop:drop-1:meta", "cached", 0)
	sizes := []stockResetSize{
		{Label: "S", Available: 2},
		{Label: "M", Available: 4},
	}
	if err := replaceStockInRedis(ctx, client, "drop-1", 6, sizes); err != nil {
		t.Fatal(err)
	}

	if total := client.Get(ctx, "drop:drop-1:stock").Val(); total != "6" {
		t.Fatalf("total stock = %s, want 6", total)
	}
	if small := client.Get(ctx, "drop:drop-1:size:S:stock").Val(); small != "2" {
		t.Fatalf("S stock = %s, want 2", small)
	}
	if medium := client.Get(ctx, "drop:drop-1:size:M:stock").Val(); medium != "4" {
		t.Fatalf("M stock = %s, want 4", medium)
	}
	if client.Exists(ctx, "drop:drop-1:meta").Val() != 0 {
		t.Fatal("drop metadata cache was not invalidated")
	}
}
