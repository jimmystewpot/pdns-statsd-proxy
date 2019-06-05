package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// DNSClient ...
type DNSClient struct {
	Host   string
	APIKey string
	C      *http.Client
}

// PDNSStat incoming statistics type
type PDNSStat struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// NewPdnsClient returns a powerdns client.
func NewPdnsClient(config *Config) *DNSClient {
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    *config.interval * 4,
		DisableCompression: true,
	}
	host := fmt.Sprintf("http://%s:%d/api/v1/servers/localhost/statistics", *config.pdnsHost, *config.pdnsPort)
	return &DNSClient{
		Host:   host,
		APIKey: *config.pdnsAPIKey,
		C:      &http.Client{Transport: transport},
	}
}

// DNSWorker wraps a ticker for task execution.
func DNSWorker(config *Config, c *DNSClient) {
	log.Info("Starting PowerDNS statistics worker...")
	interval := time.NewTicker(*config.interval)
	for {
		select {
		case <-interval.C:
			err := c.Poll(config)
			if err != nil {
				log.Warn("unable to poll PowerDNS in DNSWorker",
					zap.Error(err),
				)
			}
		case <-config.Done:
			log.Warn("done closed, exiting from DNSWorker.")
			return
		}
	}
}

// Poll for statistics
func (c *DNSClient) Poll(config *Config) error {
	request, err := http.NewRequest("GET", c.Host, nil)
	if err != nil {
		log.Fatal("error in powerdns client request",
			zap.Error(err),
		)
	}
	request.Header.Add("X-API-Key", c.APIKey)
	request.Header.Add("User-Agent", provider)

	response, err := c.C.Do(request)
	if err != nil {
		log.Fatal("error in powerdns client request",
			zap.Int("status_code", response.StatusCode),
			zap.Error(err),
		)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		log.Warn("non http status ok returned")
		return fmt.Errorf(fmt.Sprintf("status_code %d returned from PowerDNS", response.StatusCode))
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal("error reading response body",
			zap.Error(err),
		)
	}

	tmp := []PDNSStat{}
	err = json.Unmarshal(body, &tmp)
	if err != nil {
		log.Warn("unable to unmarshal json from powerdns Poll()",
			zap.Error(err),
		)
		return err
	}

	log.Info("successfully fetched PowerDNS statistics")
	for _, stat := range tmp {
		val, err := strconv.ParseInt(stat.Value, 10, 64)
		if err != nil {
			log.Warn("unable to convert value string to int64 in Poll()")
			continue
		}
		if _, ok := counter_cumulative_names[stat.Name]; ok {
			config.StatsChan <- Statistic{
				Name:  stat.Name,
				Type:  "counter_cumulative",
				Value: val,
			}
		}
		if _, ok := gauge_names[stat.Name]; ok {
			config.StatsChan <- Statistic{
				Name:  stat.Name,
				Type:  "gauge",
				Value: val,
			}
		}
	}

	return nil
}
