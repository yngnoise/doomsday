package drop

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

var replaceStockScript = redis.NewScript(`
redis.call('SET', KEYS[1], ARGV[1])
redis.call('DEL', KEYS[2])

for i = 3, #KEYS do
  redis.call('SET', KEYS[i], ARGV[i - 1])
end

return #KEYS - 2
`)

type stockResetSize struct {
	Label     string
	Available int
	Baseline  int
}

func distributeAvailableStock(labels []string, consumed map[string]int, available int) []stockResetSize {
	if len(labels) == 0 {
		return nil
	}

	perSize := available / len(labels)
	remainder := available % len(labels)
	result := make([]stockResetSize, 0, len(labels))
	for index, label := range labels {
		sizeAvailable := perSize
		if index == len(labels)-1 {
			sizeAvailable += remainder
		}
		result = append(result, stockResetSize{
			Label:     label,
			Available: sizeAvailable,
			Baseline:  sizeAvailable + consumed[label],
		})
	}
	return result
}

func replaceStockInRedis(ctx context.Context, client *redis.Client, dropID string, total int, sizes []stockResetSize) error {
	keys := []string{
		fmt.Sprintf("drop:%s:stock", dropID),
		fmt.Sprintf("drop:%s:meta", dropID),
	}
	args := []any{total}
	for _, size := range sizes {
		keys = append(keys, fmt.Sprintf("drop:%s:size:%s:stock", dropID, size.Label))
		args = append(args, size.Available)
	}

	if err := replaceStockScript.Run(ctx, client, keys, args...).Err(); err != nil {
		return fmt.Errorf("replace Redis stock: %w", err)
	}
	return nil
}
