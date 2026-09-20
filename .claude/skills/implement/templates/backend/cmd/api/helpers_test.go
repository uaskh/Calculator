package main

import (
	"testing"

	"example.com/service/internal/config"
)

func defaultConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
