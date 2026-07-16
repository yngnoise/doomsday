package drop

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

var releaseReservationScript = redis.NewScript(`
local size_key  = KEYS[1]
local total_key = KEYS[2]
local resv_key  = KEYS[3]
local user_id   = ARGV[1]
local resv_id   = ARGV[2]

local current_resv = redis.call('HGET', resv_key, user_id)
local total_stock = tonumber(redis.call('GET', total_key)) or 0

if current_resv ~= resv_id then
  return {0, total_stock}
end

redis.call('INCR', size_key)
total_stock = redis.call('INCR', total_key)
redis.call('HDEL', resv_key, user_id)

return {1, total_stock}
`)

type reservationRelease struct {
	Released   bool
	TotalStock int64
}

// releaseReservationInRedis restores both stock counters exactly once.
// The reservation hash acts as the idempotency marker: a retry after a
// successful release is a no-op, even when PostgreSQL still says pending.
func releaseReservationInRedis(
	ctx context.Context,
	rdb *redis.Client,
	dropID, size, userID, reservationID string,
) (reservationRelease, error) {
	sizeKey := fmt.Sprintf("drop:%s:size:%s:stock", dropID, size)
	totalKey := fmt.Sprintf("drop:%s:stock", dropID)
	reservationsKey := fmt.Sprintf("drop:%s:reservations", dropID)

	values, err := releaseReservationScript.Run(
		ctx,
		rdb,
		[]string{sizeKey, totalKey, reservationsKey},
		userID,
		reservationID,
	).Slice()
	if err != nil {
		return reservationRelease{}, fmt.Errorf("release reservation: %w", err)
	}
	if len(values) != 2 {
		return reservationRelease{}, fmt.Errorf("release reservation: unexpected result length %d", len(values))
	}

	released, err := luaInt64(values[0])
	if err != nil {
		return reservationRelease{}, fmt.Errorf("release reservation flag: %w", err)
	}
	totalStock, err := luaInt64(values[1])
	if err != nil {
		return reservationRelease{}, fmt.Errorf("release reservation stock: %w", err)
	}

	return reservationRelease{Released: released == 1, TotalStock: totalStock}, nil
}

func luaInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case string:
		var parsed int64
		if _, err := fmt.Sscan(v, &parsed); err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unexpected Lua integer type %T", value)
	}
}
