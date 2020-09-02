package main

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"
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
	<-config.Done
}

func Test_main(t *testing.T) {
	tests := []struct {
		name string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			main()
		})
	}
}

// we init this only for testing to initalise the logging configuration.
func init() {
	debug := getEnvStr("DEBUG", "")
	if *debug == "" {
		log = zap.NewNop()
	} else {
		log = zap.NewExample(zap.AddCaller(), zap.WithCaller(true)).Named(provider)
	}
}
