package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/morrisonbrett/SummerRateChecker/internal/bot"
	"github.com/morrisonbrett/SummerRateChecker/internal/config"
	"github.com/morrisonbrett/SummerRateChecker/internal/monitor"
	"github.com/morrisonbrett/SummerRateChecker/internal/storage"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	sugar := logger.Sugar()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	sugar.Info("SummerRateChecker starting up")

	// Initialize storage with persistence
	store, err := storage.NewFileStorage("data")
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	sugar.Info("Initialized persistent storage")

	// Initialize Discord bot
	discordBot, err := bot.New(cfg, store, sugar)
	if err != nil {
		log.Fatalf("Failed to create Discord bot: %v", err)
	}

	// Start Discord bot
	err = discordBot.Start()
	if err != nil {
		log.Fatalf("Failed to start Discord bot: %v", err)
	}
	defer discordBot.Stop()

	// Initialize and start monitor
	rateMonitor := monitor.New(cfg, store, sugar)
	rateMonitor.SetCheckTrigger(discordBot.GetCheckTrigger())

	// Start the monitoring loop
	go rateMonitor.Start()

	sugar.Info("SummerRateChecker is now running. Press CTRL-C to exit.")

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	sugar.Info("Shutting down SummerRateChecker")
}
