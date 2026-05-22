package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/secwager/secwager/market/internal/service"
)

func main() {
	cfg := service.Config{
		Brokers:    strings.Split(mustEnv("KAFKA_BROKERS"), ","),
		Topic:      envOr("KAFKA_IN_TOPIC", "incoming_orders"),
		Partition:  mustInt(mustEnv("MARKET_CARDINAL")),
		ExecTopic:  envOr("KAFKA_EXEC_TOPIC", "order_executions"),
		DepthTopic: envOr("KAFKA_DEPTH_TOPIC", "depth_updates"),
		DataDir:    envOr("MARKET_DATA_DIR", "/data"),
		FlushEvery: optInt(envOr("MARKET_FLUSH_EVERY", "1000")),
	}

	svc := service.New(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := svc.Run(ctx); err != nil {
		log.Fatalf("market: %v", err)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustInt(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		log.Fatalf("invalid integer %q: %v", s, err)
	}
	return n
}

func optInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
