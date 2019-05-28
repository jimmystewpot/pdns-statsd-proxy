package main

import (
	"os"
	"time"
)

// Config holds all the configuration required to start the service.
type Config struct {
	statsHost  *string
	statsPort  *int
	interval   *time.Duration
	pdnsHost   *string
	pdnsAPIKey *string
	recursor   *bool
	StatsChan  chan Statistic
	Done       chan bool
}

func getEnvStr(name string, def string) *string {
	content, found := os.LookupEnv(name)
	if found {
		return &content
	}
	return &def
}

func validateConfiguration(config *Config, interval *int) bool {
	if *config.statsHost == "" {
		return false
	}
	if *config.pdnsAPIKey == "" {
		config.pdnsAPIKey = getEnvStr("PDNS_API_KEY", "")
		if *config.pdnsAPIKey == "" {
			log.Warn("unable to find PowerDNS API key via flags or environment variable PDNS_API_KEY")
			return false
		}
	}
	config.interval = timePtr(time.Duration(*interval) * time.Second)

	return true
}

func timePtr(t time.Duration) *time.Duration {
	return &t
}
