package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/notes-bin/ibed/internal/api"
	"github.com/notes-bin/ibed/internal/cache"
	"github.com/notes-bin/ibed/internal/config"
	"github.com/notes-bin/ibed/internal/redis"
)

func main() {
	// 初始化日志
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// 加载配置文件
	configFile, err := os.ReadFile("config/config.json")
	if err != nil {
		slog.Error("Failed to read config", "error", err)
		os.Exit(1)
	}
	var cfg config.Config
	if err := json.Unmarshal(configFile, &cfg); err != nil {
		slog.Error("Failed to parse config", "error", err)
		os.Exit(1)
	}

	// 初始化 Redis
	redisClient, err := redis.NewClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, cfg.Redis.PoolSize)
	if err != nil {
		slog.Error("Failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	// 初始化 Top10 缓存
	go cache.StartTop10Refresh(context.Background(), redisClient, cfg.TopRefreshInterval)

	// 设置路由
	router := api.SetupRouter(&cfg, redisClient)

	// 启动服务器
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}
	go func() {
		slog.Info("Server starting on port", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server shutdown failed", "error", err)
		os.Exit(1)
	}
	slog.Info("Server stopped")
}
