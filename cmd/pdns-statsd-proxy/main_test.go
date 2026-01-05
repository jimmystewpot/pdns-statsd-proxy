package main

import (
	"os"
	"testing"
	"time"
)

func TestWatchSignals(t *testing.T) {
	config := testConfig()

	sigs := make(chan os.Signal, 1)
	go watchSignals(sigs, config)

	sigs <- os.Interrupt

	select {
	case <-config.stop:
		return
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for stop")
	}
}

func TestWatchSignals_StopOnce(t *testing.T) {
	config := testConfig()

	sigA := make(chan os.Signal, 1)
	sigB := make(chan os.Signal, 1)

	go watchSignals(sigA, config)
	go watchSignals(sigB, config)

	sigA <- os.Interrupt
	sigB <- os.Interrupt

	select {
	case <-config.stop:
		return
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for stop")
	}
}
