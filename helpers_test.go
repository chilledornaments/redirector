//go:build unit_test || load_test

package main

import (
	"log/slog"
)

var testLogger *slog.Logger

func newTestLogger() *slog.Logger {
	if testLogger == nil {
		testLogger = NewLogger(slog.LevelDebug, true)
	}
	return testLogger
}

func newTestRules(p string) RuleMapping {
	r, err := loadConfig(newTestLogger(), p)

	if err != nil {
		panic(err)
	}

	return r.RuleMap
}
