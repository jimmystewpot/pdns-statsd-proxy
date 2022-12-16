package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// pdnsClient stores the configuration for the powerdns client.
type pdnsClient struct {
	Host   string
	APIKey string
	Client *http.Client
}

// pdnsStat incoming statistics type
type pdnsStat struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	Size  interface{} `json:"size"`
	Value interface{} `json:"value"`
}

// Initialise a new powerdns client.
func (pdns *pdnsClient) Initialise(config *Config) {
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    *config.interval * 4,
		DisableCompression: true,
	}

	pdns.Host = fmt.Sprintf("http://%s/api/v1/servers/localhost/statistics", net.JoinHostPort(*config.pdnsHost, *config.pdnsPort))

	pdns.APIKey = *config.pdnsAPIKey
	pdns.Client = &http.Client{Transport: transport}
}

// Worker wraps a ticker for task execution to query the powerdns API.
func (pdns *pdnsClient) Worker(config *Config) {
	log.Info("Starting PowerDNS statistics worker...")
	interval := time.NewTicker(*config.interval)
	for {
		select {
		case <-interval.C:
			response, err := pdns.Poll()
			if err != nil {
				log.Error("powerdns client",
					zap.Error(err),
				)
				continue
			}
			err = decodeStats(response, config)
			if err != nil {
				log.Error("powerdns decodeStats",
					zap.Error(err),
				)
			}
		case <-config.pdnsDone:
			log.Info("exiting from pdns Worker.")
			close(config.pdnsDone)
			return
		}
	}
}

// Poll for statistics
func (pdns *pdnsClient) Poll() (*http.Response, error) {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				log.Error("recovered from panic in pdnsClient.Polll()",
					zap.Error(err),
				)
			}
		}
	}()

	request, err := http.NewRequest("GET", pdns.Host, http.NoBody)
	if err != nil {
		return &http.Response{}, fmt.Errorf("unable to instantiate new http client: %s", err)
	}

	request.Header.Add("X-API-Key", pdns.APIKey)
	request.Header.Add("User-Agent", provider)

	response, err := pdns.Client.Do(request)
	if err != nil {
		return &http.Response{}, err
	}

	if response.StatusCode != http.StatusOK {
		return &http.Response{}, fmt.Errorf("expected status_code %d got %d returned from PowerDNS", http.StatusOK, response.StatusCode)
	}

	log.Info("successfully queried PowerDNS statistics")

	return response, nil
}

func decodeStats(response *http.Response, config *Config) error {
	defer response.Body.Close()

	stats := make([]pdnsStat, 0)

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &stats)
	if err != nil {
		return err
	}

	for i := 0; i < len(stats); i++ {
		switch stats[i].Type {
		case "StatisticItem":
			err = decodeStringStat(response, stats[i], config, false)
			if err != nil {
				return err
			}
		case "MapStatisticItem": // adds the new MapStatisticsItem type added in 4.2.0
			for _, stat := range stats[i].Value.([]interface{}) {
				if m, ok := stat.(map[string]interface{}); !ok {
					continue
				} else {
					val, err := strconv.ParseInt(m["value"].(string), 10, 64)
					fmt.Println(stat)
					if err != nil {
						return fmt.Errorf("unable to convert %s string to int64 in decodeStats()", m["value"])
					}
					n := fmt.Sprintf("%s-%s", stats[i].Name, m["name"])
					// populate the map with metrics names.
					if _, ok := config.counterCumulativeValues[n]; !ok {
						config.counterCumulativeValues[n] = -1
					}
					config.StatsChan <- Statistic{
						Name:  n,
						Type:  counterCumulative,
						Value: val,
					}
				}
			}
		case "RingStatisticItem":
			if str, ok := stats[i].Size.(string); ok {
				val, err := strconv.ParseInt(str, 10, 64)
				if err != nil {
					return fmt.Errorf("unable to convert %s value string to int64 in decodeStats()", str)
				}
				// populate the map with metrics names.
				if _, ok := config.counterCumulativeValues[stats[i].Name]; !ok {
					config.counterCumulativeValues[stats[i].Name] = -1
				}
				config.StatsChan <- Statistic{
					Name:  stats[i].Name,
					Type:  gauge,
					Value: val,
				}
			}
		default:
			// this allows for forward compatibility of powerdns adds new metrics types we just skip over them.
			// we emit a metric so that we know this is happening. powerdns.(recursor|authoritative).unknown.(type)
			err := decodeStringStat(response, stats[i], config, true)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeStringStat(response *http.Response, stat pdnsStat, config *Config, unknown bool) error {
	var n string
	if str, ok := stat.Value.(string); ok {
		val, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return fmt.Errorf("unable to convert %s value string to int64 in decodeStats()", str)
		}
		switch unknown {
		case true:
			n = fmt.Sprintf("unknown.%s", stat.Type)
		default:
			n = stat.Name
		}

		// populate the map with metrics names.
		if _, ok := config.counterCumulativeValues[n]; !ok {
			config.counterCumulativeValues[n] = -1
		}

		config.StatsChan <- Statistic{
			Name:  n,
			Type:  counterCumulative,
			Value: val,
		}
		version := response.Header.Get("Server")
		log.Info("unknown metric type in api response",
			zap.String("pdns_version", version),
			zap.String("type", stat.Type),
			zap.String("name", stat.Name),
			zap.Int64("value", val),
		)
	}
	return nil
}
