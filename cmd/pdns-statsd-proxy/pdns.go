package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
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
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// NewPdnsClient returns a powerdns client.
func NewPdnsClient(config *Config) *DNSClient {
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    *config.interval * 4,
		DisableCompression: true,
	}

	hostPort := net.JoinHostPort(*config.pdnsHost, *config.pdnsPort)

	host := fmt.Sprintf("http://%s/api/v1/servers/localhost/statistics", hostPort)

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
			response, err := c.Poll(config)
			if err != nil {
				log.Warn("powerdns client",
					zap.Error(err),
				)
			}
			err = decodeStats(response, config)
			if err != nil {
				log.Warn("powerdns decodeStats",
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
func (c *DNSClient) Poll(config *Config) (*http.Response, error) {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				log.Info("recovered from panic",
					zap.Error(err),
				)
			}
		}
	}()

	request, err := http.NewRequest("GET", c.Host, nil)
	if err != nil {
		return &http.Response{}, fmt.Errorf("unable to instantiate new http client: %s", err)
	}

	request.Header.Add("X-API-Key", c.APIKey)
	request.Header.Add("User-Agent", provider)

	response, err := c.C.Do(request)
	if err != nil {
		return &http.Response{}, err
	}

	if response.StatusCode != http.StatusOK {
		return &http.Response{}, fmt.Errorf(fmt.Sprintf("expected status_code %d got %d returned from PowerDNS", http.StatusOK, response.StatusCode))
	}

	log.Info("successfully queried PowerDNS statistics")

	return response, nil
}

func decodeStats(response *http.Response, config *Config) error {
	defer response.Body.Close()

	stats := make([]PDNSStat, 0)

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &stats)
	if err != nil {
		return err
	}

	for _, stat := range stats {
		switch stat.Type {
		case "StatisticItem":
			if str, ok := stat.Value.(string); ok {
				val, err := strconv.ParseInt(str, 10, 64)
				if err != nil {
					log.Warn("unable to convert value string to int64 in Poll()")
					continue
				}
				if _, ok := gaugeNames[stat.Name]; ok {
					config.StatsChan <- Statistic{
						Name:  stat.Name,
						Type:  gauge,
						Value: val,
					}
					continue
				}

				// populate the map with metrics names.
				if _, ok := counterCumulativeValues[stat.Name]; !ok {
					counterCumulativeValues[stat.Name] = -1
				}

				config.StatsChan <- Statistic{
					Name:  stat.Name,
					Type:  counterCumulative,
					Value: val,
				}
			}
		case "MapStatisticItem": // adds the new MapStatisticsItem type added in 4.2.0

			for _, i := range stat.Value.([]interface{}) {

				if m, ok := i.(map[string]interface{}); ok {
					val, err := strconv.ParseInt(m["value"].(string), 10, 64)
					if err != nil {
						return fmt.Errorf("unable to convert %s string to int64 in decodeStats()", m["value"])
					}
					config.StatsChan <- Statistic{
						Name:  fmt.Sprintf("%s-%s", stat.Name, m["name"]),
						Type:  counterCumulative,
						Value: val,
					}
				}
			}
		default:
			continue
		}
	}
	return nil
}
