//go:build load_test

package main

import (
	"testing"
	"time"
)

func Test_loadHugeConfig(t *testing.T) {
	f := "./fixtures/load_test.yml"
	start := time.Now()

	_, err := loadConfig(newTestLogger(), f)
	if err != nil {
		t.Fatal(err)
	}

	end := time.Now()
	d := end.Sub(start).Milliseconds()
	t.Logf("load test took %dms", d)

	//
	if d > 1000 {
		t.Error("loading huge config took too long")
	}
}
