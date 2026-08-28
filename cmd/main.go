package main

import (
	"CryptoRecordBot/internal/bootstrap"
	"CryptoRecordBot/internal/config"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	app, err := bootstrap.NewApp(cfg)
	if err != nil {
		return fmt.Errorf("initialization error: %w", err)
	}

	if err := app.Run(ctx); err != nil {
		slog.Error("Application terminated with error", slog.Any("error", err))
		return err
	}

	slog.Info("Application gracefully stopped.")
	return nil
}
