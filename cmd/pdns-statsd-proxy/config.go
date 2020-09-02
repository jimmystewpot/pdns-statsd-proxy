package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

// Config holds all the configuration required to start the service.
type Config struct {
	statsHost  *string
	statsPort  *string
	interval   *time.Duration
	pdnsHost   *string
	pdnsPort   *string
	pdnsAPIKey *string
	recursor   *bool
	StatsChan  chan Statistic
	Done       chan bool
}

// validateConfiguration confirms that the basic configuration parameters are correctly set.
func validateConfiguration(config *Config) bool {
	config.statsHost = flag.String("statsHost", "127.0.0.1", "The statsd server to emit metrics")
	config.statsPort = flag.String("statsPort", "8125", "The port that statsd is listening on")
	config.recursor = flag.Bool("recursor", true, "Query recursor statistics")
	config.pdnsHost = flag.String("pdnsHost", "127.0.0.1", "The host to query for powerdns statistics")
	config.pdnsPort = flag.String("pdnsPort", "8080", "The port that PowerDNS API is accepting connections")
	config.pdnsAPIKey = flag.String("key", "", "The api key for the powerdns api")
	interval := flag.Int("interval", 15, "The interval to emit metrics in seconds")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] \n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	config.StatsChan = make(chan Statistic, 1000)
	config.Done = make(chan bool, 1)

	config.interval = timePtr(time.Duration(*interval) * time.Second)

	if !checkStatsHost(config) {
		return false
	}
	if !checkpdnsAPIKey(config) {
		return false
	}
	return true
}

func checkStatsHost(config *Config) bool {
	if *config.statsHost == "" {
		log.Warn("unable to find the statsd host to send metrics to")
		return false
	}
	return true
}

func checkpdnsAPIKey(config *Config) bool {
	if *config.pdnsAPIKey == "" {
		// check if its in the environment variables list.
		config.pdnsAPIKey = getEnvStr("PDNS_API_KEY", "")
		// if its still empty we can't start.
		if *config.pdnsAPIKey == "" {
			log.Warn("unable to find PowerDNS API key via flags or environment variable PDNS_API_KEY")
			return false
		}
	}
	// the key is not empty we should be able to start.
	return true
}

// getEnvStr looks up an environment variable or returns the default value.
func getEnvStr(name string, def string) *string {
	content, found := os.LookupEnv(name)
	if found {
		return &content
	}
	return &def
}

// timePtr returns a pointer for Time.Duration.
func timePtr(t time.Duration) *time.Duration {
	return &t
}

// stringPtr returns a pointer for an input string
func stringPtr(s string) *string {
	return &s
}

// boolPtr returns a pointer for an input boolean
func boolPtr(b bool) *bool {
	return &b
}
