package sales_test

import (
	"fmt"

	"github.com/redis/go-redis/v9"
)

func redisZ(i int) redis.Z { return redis.Z{Score: float64(i + 1), Member: fmt.Sprintf("q%06d", i)} }
