package main

import (
	"log/slog"
	"os"
)

func NewLogger(level slog.Level, src bool) *slog.Logger {
	opts := &slog.HandlerOptions{
		AddSource:   src,
		Level:       level,
		ReplaceAttr: nil,
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
