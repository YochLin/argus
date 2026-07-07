package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/joho/godotenv"
	"argus/internal/bot"
	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/scheduler"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading env from environment")
	}

	telegramToken := mustEnv("TELEGRAM_BOT_TOKEN")
	chatIDStr := mustEnv("TELEGRAM_CHAT_ID")
	finnhubKey := os.Getenv("FINNHUB_API_KEY")
	recommendModel := envOr("CLAUDE_RECOMMEND_MODEL", "opus")
	checkModel := envOr("CLAUDE_CHECK_MODEL", "sonnet")
	chatModel := envOr("CLAUDE_CHAT_MODEL", "sonnet")
	dbPath := envOr("DB_PATH", "data/argus.db")
	lang := i18n.Parse(envOr("BOT_LANGUAGE", "zh"))

	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		log.Fatalf("invalid TELEGRAM_CHAT_ID: %v", err)
	}

	// Ensure DB directory exists
	if err := os.MkdirAll("data", 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	// Set up multi-provider data layer (Finnhub primary, Yahoo fallback).
	// Fundamentals/financial statements are Finnhub-only (Yahoo's
	// equivalent requires a crumb/cookie handshake we don't implement), so
	// fundamentalsProvider stays nil when no Finnhub key is configured.
	// Historical closes (for RSI/MACD) go the other way: Finnhub's free
	// tier blocks /stock/candle entirely, so history is Yahoo-only.
	var providers []data.Provider
	var fundamentalsProvider data.FundamentalsProvider
	if finnhubKey != "" {
		finnhub := data.NewFinnhub(finnhubKey)
		providers = append(providers, finnhub)
		fundamentalsProvider = finnhub
	}
	yahoo := data.NewYahoo()
	providers = append(providers, yahoo)
	provider := data.NewMulti(providers...)

	llmClient := llm.NewClient(recommendModel, checkModel, chatModel, lang)
	defer llmClient.Close() // kills any still-open persistent chat session's subprocess

	telegramBot, err := bot.New(telegramToken, chatID, database, provider, fundamentalsProvider, yahoo, llmClient, lang)
	if err != nil {
		log.Fatalf("init bot: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sched := scheduler.New()
	sched.AddDailyReport(ctx, func(ctx context.Context) {
		telegramBot.RunDailyReport(ctx)
	})
	sched.AddClosingSnapshot(ctx, func(ctx context.Context) {
		telegramBot.RunClosingSnapshot(ctx)
	})
	sched.Start()
	defer sched.Stop()

	log.Println("stock trader bot started")
	telegramBot.Run(ctx)
	log.Println("bot stopped")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
