package main

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func TestWatchSignals(t *testing.T) {
	config := testConfig()

	stats, _ = NewStatsClient(config)
	sigs := make(chan os.Signal, 1)

	go watchSignals(sigs, config)
	signal.Notify(sigs, syscall.SIGINT)
	go func() {
		time.Sleep(50 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()

	// this should block until the signal is received by the watch signals, otherwise the test fails.
	<-config.done
}
