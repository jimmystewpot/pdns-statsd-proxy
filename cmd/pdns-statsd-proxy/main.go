package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

const (
	provider string = "pdns-stats-proxy"
	// metric types
	counterCumulative string        = "counter_cumulative"
	gauge             string        = "gauge"
	statsBufferSize   int           = 1000
	defaultInterval   time.Duration = 15 * time.Second
	delayMultipler    int           = 4
)

var (
	log   *zap.Logger
	stats statsClient

	// flag variables set as globals allows us to test various types of flags without needing to hack around
	// the flags package.
	configFile = flag.String("config", "/etc/pdns-statsd-proxy/config.yaml", "Path to YAML config file")
	statsHost  = flag.String("statsHost", "127.0.0.1", "The statsd server to emit metrics")
	statsPort  = flag.String("statsPort", "8125", "The port that statsd is listening on")
	pdnsHost   = flag.String("pdnsHost", "127.0.0.1", "The host to query for powerdns statistics")
	pdnsPort   = flag.String("pdnsPort", "8080", "The port that PowerDNS API is accepting connections")
	pdnsAPIKey = flag.String("key", "", "The api key for the powerdns api")
	recursor   = flag.Bool("recursor", true, "Query recursor statistics")
	histograms = flag.Bool("histograms", false,
		"Emit Prometheus histogram metrics (count/sum). "+
			"Disabled by default because histogram support varies across statsd backends",
	)
	interval = flag.Duration("interval", defaultInterval, "The interval to emit metrics in seconds")
)

// handle a graceful exit so that we do not lose data when we restart the service.
//

func watchSignals(sig <-chan os.Signal, config *Config) {
	<-sig
	log.Info("Caught signal about to cleanly exit.")
	config.stopOnce.Do(func() { close(config.stop) })
}

var (
	errInvalidConfig   = errors.New("invalid configuration")
	errStatsClientInit = errors.New("unable to initiate statsd client")
)

type mainDeps struct {
	validateConfig func(*Config) bool
	newStatsClient func(*Config) (statsClient, error)
	startPDNS      func(*Config)
	startStats     func(*Config)
	notifySignals  func(chan<- os.Signal, ...os.Signal)
}

func runMainWithDeps(config *Config, deps mainDeps) error {
	if !deps.validateConfig(config) {
		return errInvalidConfig
	}

	// initiate the statsd client.
	var err error
	stats, err = deps.newStatsClient(config)
	if err != nil {
		return errStatsClientInit
	}

	deps.startPDNS(config)
	deps.startStats(config)

	// handle signals correctly.
	sigs := make(chan os.Signal, 1)
	deps.notifySignals(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	go watchSignals(sigs, config)

	// wait until stop is triggered
	<-config.stop
	// wait for workers to exit
	<-config.pdnsExited
	<-config.statsExited
	return nil
}

func main() {
	err := initLogger()
	if err != nil {
		fmt.Println("unable to initialise logging: ", err)
		os.Exit(1)
	}

	config := new(Config)
	deps := mainDeps{
		validateConfig: (*Config).Validate,
		newStatsClient: NewStatsClient,
		startPDNS: func(c *Config) {
			pdnsClient := new(pdnsClient)
			pdnsClient.Initialise(c)
			go pdnsClient.Worker(c)
		},
		startStats: func(c *Config) {
			go statsWorker(c)
		},
		notifySignals: signal.Notify,
	}
	err = runMainWithDeps(config, deps)
	switch {
	case errors.Is(err, errInvalidConfig):
		log.Fatal("Unable to process configuration, missing flags")
	case errors.Is(err, errStatsClientInit):
		log.Fatal("Unable to initiate statsd client")
	case err != nil:
		log.Fatal("Unable to start")
	}
}
