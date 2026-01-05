package main

import (
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Config holds all the configuration required to start the service.
type Config struct {
	statsHost               *string
	statsPort               *string
	interval                *time.Duration
	pdnsHost                *string
	pdnsPort                *string
	pdnsAPIKey              *string
	recursor                *bool
	counterCumulativeValues map[string]int64
	StatsChan               chan Statistic
	stopOnce                sync.Once
	stop                    chan struct{} // close global stop signal
	pdnsExited              chan struct{} // closed by the pdns worker
	statsExited             chan struct{} // closed by the stats worker
}

func (c *Config) flags() bool {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] \n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if c.statsHost == nil {
		c.statsHost = statsHost
	}
	c.statsPort = statsPort
	c.pdnsHost = pdnsHost
	c.pdnsPort = pdnsPort
	// Ensure pdnsAPIKey is always initialised (tests may set it directly).
	if c.pdnsAPIKey == nil {
		c.pdnsAPIKey = pdnsAPIKey
	}
	// If the flag value is empty, fallback to environment variable.
	if c.pdnsAPIKey == nil || *c.pdnsAPIKey == "" {
		c.pdnsAPIKey = getEnvStr("PDNS_API_KEY", "")
	}
	c.recursor = recursor
	c.interval = interval

	return flag.Parsed()
}

// Validate the configuration is correct before starting the service.
func (c *Config) Validate() bool {
	if !c.flags() {
		return false
	}

	err := c.CheckStatsHost()
	if err != nil {
		log.Error("CheckStatsHost",
			zap.Error(err),
		)
		return false
	}

	err = c.CheckAPIKey()
	if err != nil {
		log.Error("checkdnsAPIKey",
			zap.Error(err),
		)
		return false
	}

	// configuration is all okay, initialise the remaining internals
	c.counterCumulativeValues = make(map[string]int64)

	c.StatsChan = make(chan Statistic, statsBufferSize)
	c.stop = make(chan struct{})
	c.pdnsExited = make(chan struct{})
	c.statsExited = make(chan struct{})
	return true
}

func (c *Config) CheckStatsHost() error {
	if *c.statsHost == "" {
		return fmt.Errorf("no statsd host specified in the configuration")
	}
	return nil
}

func (c *Config) CheckAPIKey() error {
	if *c.pdnsAPIKey == "" {
		return fmt.Errorf("unable to find PowerDNS API key via flags or environment variable PDNS_API_KEY")
	}
	// the key is not empty we should be able to start.
	return nil
}

// getEnvStr looks up an environment variable or returns the default value.
func getEnvStr(name string, def string) *string {
	content, found := os.LookupEnv(name)
	if found {
		return &content
	}
	return &def
}
