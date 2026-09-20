package main

import (
	"testing"

	"github.com/aksh/calculator/backend/internal/config"
)

func defaultConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
