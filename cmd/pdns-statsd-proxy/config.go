package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Config holds all the configuration required to start the service.
type Config struct {
	counterCumulativeValues map[string]int64
	configFile              *string
	statsHost               *string
	statsPort               *string
	interval                *time.Duration
	pdnsHost                *string
	pdnsPort                *string
	pdnsAPIKey              *string
	recursor                *bool
	histograms              *bool
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

	setFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	if c.configFile == nil {
		c.configFile = configFile
	}
	if c.configFile != nil && *c.configFile != "" {
		if !setFlags["config"] {
			if _, err := os.Stat(*c.configFile); err != nil {
				if os.IsNotExist(err) {
					goto afterYAML
				}
				log.Error("unable to stat yaml config",
					zap.String("path", *c.configFile),
					zap.Error(err),
				)
				return false
			}
		}
		err := applyYAMLConfig(*c.configFile, setFlags)
		if err != nil {
			log.Error("unable to load yaml config",
				zap.String("path", *c.configFile),
				zap.Error(err),
			)
			return false
		}
	}

afterYAML:

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
	c.histograms = histograms
	c.interval = interval

	return flag.Parsed()
}

type yamlConfig struct {
	StatsHost  *string `yaml:"statsHost"`
	StatsPort  *string `yaml:"statsPort"`
	Interval   *string `yaml:"interval"`
	PDNSHost   *string `yaml:"pdnsHost"`
	PDNSPort   *string `yaml:"pdnsPort"`
	APIKey     *string `yaml:"key"`
	Recursor   *bool   `yaml:"recursor"`
	Histograms *bool   `yaml:"histograms"`
}

func parseIntervalValue(raw string) (time.Duration, error) {
	if d, err := time.ParseDuration(raw); err == nil {
		return d, nil
	}
	secs, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid interval %q", raw)
	}
	return time.Duration(secs) * time.Second, nil
}

func openYAMLConfigFile(path string) (*os.File, string, error) {
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return nil, "", fmt.Errorf("config path must be absolute: %s", cleanPath)
	}
	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("config path is not a regular file: %s", cleanPath)
	}

	f, err := os.Open(cleanPath)
	if err != nil {
		return nil, "", err
	}
	return f, cleanPath, nil
}

func applyYAMLConfig(path string, setFlags map[string]bool) error {
	f, cleanPath, err := openYAMLConfigFile(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Debug("unable to close yaml config file",
				zap.String("path", cleanPath),
				zap.Error(err),
			)
		}
	}()
	buf, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	var yc yamlConfig
	if err := yaml.Unmarshal(buf, &yc); err != nil {
		return err
	}

	if yc.StatsHost != nil && !setFlags["statsHost"] {
		*statsHost = *yc.StatsHost
	}
	if yc.StatsPort != nil && !setFlags["statsPort"] {
		*statsPort = *yc.StatsPort
	}
	if yc.PDNSHost != nil && !setFlags["pdnsHost"] {
		*pdnsHost = *yc.PDNSHost
	}
	if yc.PDNSPort != nil && !setFlags["pdnsPort"] {
		*pdnsPort = *yc.PDNSPort
	}
	if yc.APIKey != nil && !setFlags["key"] {
		*pdnsAPIKey = *yc.APIKey
	}
	if yc.Recursor != nil && !setFlags["recursor"] {
		*recursor = *yc.Recursor
	}
	if yc.Histograms != nil && !setFlags["histograms"] {
		*histograms = *yc.Histograms
	}
	if yc.Interval != nil && !setFlags["interval"] {
		d, err := parseIntervalValue(*yc.Interval)
		if err != nil {
			return err
		}
		*interval = d
	}

	return nil
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
