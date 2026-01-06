package main

import (
	"errors"
	"os"
	"sync/atomic"
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

type noopStatsClient struct{}

func (n noopStatsClient) Gauge(name string, value float64, tags []string, rate float64) error {
	return nil
}
func (n noopStatsClient) Count(name string, value int64, tags []string, rate float64) error {
	return nil
}
func (n noopStatsClient) Close() error { return nil }

func TestRunMainWithDeps_InvalidConfig(t *testing.T) {
	config := testConfig()

	deps := mainDeps{
		validateConfig: func(*Config) bool { return false },
		newStatsClient: func(*Config) (statsClient, error) { return noopStatsClient{}, nil },
		startPDNS:      func(*Config) {},
		startStats:     func(*Config) {},
		notifySignals:  func(chan<- os.Signal, ...os.Signal) {},
	}

	err := runMainWithDeps(config, deps)
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("runMainWithDeps() err = %v, want %v", err, errInvalidConfig)
	}
}

func TestRunMainWithDeps_StatsClientInitFailure(t *testing.T) {
	config := testConfig()

	deps := mainDeps{
		validateConfig: func(*Config) bool { return true },
		newStatsClient: func(*Config) (statsClient, error) { return nil, errors.New("boom") },
		startPDNS:      func(*Config) {},
		startStats:     func(*Config) {},
		notifySignals:  func(chan<- os.Signal, ...os.Signal) {},
	}

	err := runMainWithDeps(config, deps)
	if !errors.Is(err, errStatsClientInit) {
		t.Fatalf("runMainWithDeps() err = %v, want %v", err, errStatsClientInit)
	}
}

func TestRunMainWithDeps_Success_WaitsForStopAndWorkersExit(t *testing.T) {
	config := testConfig()

	var pdnsStarted int32
	var statsStarted int32
	var notified int32

	deps := mainDeps{
		validateConfig: func(*Config) bool { return true },
		newStatsClient: func(*Config) (statsClient, error) { return noopStatsClient{}, nil },
		startPDNS: func(c *Config) {
			atomic.AddInt32(&pdnsStarted, 1)
		},
		startStats: func(c *Config) {
			atomic.AddInt32(&statsStarted, 1)
		},
		notifySignals: func(ch chan<- os.Signal, sig ...os.Signal) {
			atomic.AddInt32(&notified, 1)
			_ = ch
			_ = sig
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- runMainWithDeps(config, deps)
	}()

	// Ensure workers were started.
	deadline := time.After(500 * time.Millisecond)
	for atomic.LoadInt32(&pdnsStarted) == 0 || atomic.LoadInt32(&statsStarted) == 0 || atomic.LoadInt32(&notified) == 0 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for runMainWithDeps to start workers and register signals")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	// Trigger stop and worker exits.
	close(config.stop)
	close(config.pdnsExited)
	close(config.statsExited)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runMainWithDeps() err = %v, want nil", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for runMainWithDeps to return")
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
