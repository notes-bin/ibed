package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/notes-bin/ibed/internal/redis"
)

func StartTop10Refresh(ctx context.Context, redis *redis.Client, interval int) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 获取 Top10 图片 ID
			ids, err := redis.GetTop10Images(ctx)
			if err != nil {
				slog.Error("Failed to refresh Top10", "error", err)
				continue
			}
			// 缓存到 Redis（可选：直接使用 Sorted Set）
			slog.Info("Refreshed Top10 cache", "ids", ids)
		}
	}
}
