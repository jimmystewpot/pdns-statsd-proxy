package main

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func newTestConfig() *Config {
	done := make(chan bool, 1)
	config := &Config{
		Done:      done,
		statsHost: stringPtr("127.0.0.1"),
		statsPort: stringPtr("8125"),
		recursor:  boolPtr(true),
		interval:  timePtr(time.Duration(10) * time.Second),
	}
	return config
}

func TestWatchSignals(t *testing.T) {
	config := newTestConfig()

	stats, _ = NewStatsClient(config)
	sigs := make(chan os.Signal, 1)

	go watchSignals(sigs, config)
	signal.Notify(sigs, syscall.SIGINT)
	go func() {
		time.Sleep(50 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()

	// this should block until the signal is received by the watch signals, otherwise the test fails.
	<-config.Done
}
