package main

import (
	"log/slog"
	"os"

	"github.com/fly-io/162719/cmd/flyio-machine/commands"
)

func main() {
	// Initialize structured logger with text format for readability
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	commands.Execute()
}
