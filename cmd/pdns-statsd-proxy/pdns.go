package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"go.uber.org/zap"
)

const (
	unableToClosePDNSResponseBodyMsg = "unable to close PowerDNS response body"
	prometheusSampleMinFields        = 2

	serverVersionSplitParts = 4
	serverVersionMinParts   = 2
	serverVersionPatchPart  = 3

	prometheusMinMajor = 4
	prometheusMinMinor = 3
	prometheusMinPatch = 0
)

// pdnsClient stores the configuration for the powerdns client.
type pdnsClient struct {
	Client *http.Client
	APIKey string
	Host   string

	legacyPath     string
	prometheusPath string

	versionOnce         sync.Once
	serverVersion       pdnsVersion
	serverVersionParsed bool
	usePrometheus       bool
}

type pdnsVersion struct {
	major int
	minor int
	patch int
}

// pdnsStat incoming statistics type
type pdnsStat struct {
	Size  interface{} `json:"size"`
	Value interface{} `json:"value"`
	Name  string      `json:"name"`
	Type  string      `json:"type"`
}

// Initialise a new powerdns client.
//
//nolint:mnd // maxIdleConns is not a mnd
func (pdns *pdnsClient) Initialise(config *Config) {
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    time.Duration(delayMultipler) * *config.interval,
		DisableCompression: true,
	}

	baseURL := fmt.Sprintf("http://%s", net.JoinHostPort(*config.pdnsHost, *config.pdnsPort))
	pdns.legacyPath = fmt.Sprintf("%s/api/v1/servers/localhost/statistics", baseURL)
	pdns.prometheusPath = fmt.Sprintf("%s/metrics", baseURL)
	// Default to legacy until we discover the server version.
	pdns.Host = pdns.legacyPath

	pdns.APIKey = *config.pdnsAPIKey
	// Ensure polls don't hang indefinitely. Default to 10s, but for very small intervals
	// keep the timeout below the poll cadence.
	timeout := 10 * time.Second
	if config.interval != nil && *config.interval > 0 {
		candidate := *config.interval / 2
		if candidate > 0 && candidate < timeout {
			timeout = candidate
		}
	}
	// Avoid a near-zero timeout making the service unusable.
	if timeout < 100*time.Millisecond {
		timeout = 100 * time.Millisecond
	}
	pdns.Client = &http.Client{Transport: transport, Timeout: timeout}
}

// Worker wraps a ticker for task execution to query the powerdns API.
func (pdns *pdnsClient) Worker(config *Config) {
	log.Info("Starting PowerDNS statistics worker...")
	interval := time.NewTicker(*config.interval)
	defer interval.Stop()
	defer close(config.pdnsExited)
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

			closeBody := func() {
				if response != nil && response.Body != nil {
					if err := response.Body.Close(); err != nil {
						log.Debug(unableToClosePDNSResponseBodyMsg,
							zap.Error(err),
						)
					}
				}
			}
			if pdns.usePrometheus {
				err = decodePrometheusStats(response, config)
			} else {
				err = decodeStats(response, config)
			}
			closeBody()
			if err != nil {
				log.Error("powerdns decodeStats",
					zap.Error(err),
				)
			}
		case <-config.stop:
			log.Info("exiting from pdns Worker.")
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

	prometheusMinVersion := pdnsVersion{major: prometheusMinMajor, minor: prometheusMinMinor, patch: prometheusMinPatch}

	// If this is the first poll, hit the legacy endpoint to discover the server version,
	// then switch to /metrics for >=4.3.
	if !pdns.serverVersionParsed {
		return pdns.pollWithDiscovery(prometheusMinVersion)
	}

	requestURL := pdns.legacyPath
	if pdns.usePrometheus {
		requestURL = pdns.prometheusPath
	}
	request, err := http.NewRequest("GET", requestURL, http.NoBody)
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
		if err := response.Body.Close(); err != nil {
			log.Debug(unableToClosePDNSResponseBodyMsg,
				zap.Error(err),
			)
		}
		return &http.Response{}, fmt.Errorf(
			"expected status_code %d got %d returned from PowerDNS",
			http.StatusOK,
			response.StatusCode,
		)
	}

	return response, nil
}

func (pdns *pdnsClient) pollWithDiscovery(prometheusMinVersion pdnsVersion) (*http.Response, error) {
	ensurePrometheusMode := func(resp *http.Response) {
		pdns.versionOnce.Do(func() {
			version := resp.Header.Get("Server")
			if v, ok := parsePDNSServerHeader(version); ok {
				pdns.serverVersion = v
				pdns.serverVersionParsed = true
				pdns.usePrometheus = isAtLeast(v, prometheusMinVersion)
				if pdns.usePrometheus {
					pdns.Host = pdns.prometheusPath
				} else {
					pdns.Host = pdns.legacyPath
				}
			}
		})
	}

	resp, err := pdns.doRequest(pdns.legacyPath)
	if err != nil {
		return &http.Response{}, err
	}
	ensurePrometheusMode(resp)
	if pdns.usePrometheus {
		if err := resp.Body.Close(); err != nil {
			log.Debug(unableToClosePDNSResponseBodyMsg,
				zap.Error(err),
			)
		}
		return pdns.doRequest(pdns.prometheusPath)
	}
	return resp, nil
}

func (pdns *pdnsClient) doRequest(url string) (*http.Response, error) {
	request, err := http.NewRequest("GET", url, http.NoBody)
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
		if err := response.Body.Close(); err != nil {
			log.Debug(unableToClosePDNSResponseBodyMsg,
				zap.Error(err),
			)
		}
		return &http.Response{}, fmt.Errorf(
			"expected status_code %d got %d returned from PowerDNS",
			http.StatusOK,
			response.StatusCode,
		)
	}
	return response, nil
}

func parsePDNSServerHeader(server string) (pdnsVersion, bool) {
	idx := strings.Index(server, "PowerDNS/")
	if idx < 0 {
		return pdnsVersion{}, false
	}
	ver := strings.TrimPrefix(server[idx:], "PowerDNS/")
	ver = strings.TrimSpace(ver)
	if ver == "" {
		return pdnsVersion{}, false
	}
	parts := strings.SplitN(ver, ".", serverVersionSplitParts)
	if len(parts) < serverVersionMinParts {
		return pdnsVersion{}, false
	}

	major, err := strconv.Atoi(readNumericPrefix(parts[0]))
	if err != nil {
		return pdnsVersion{}, false
	}
	minor, err := strconv.Atoi(readNumericPrefix(parts[1]))
	if err != nil {
		return pdnsVersion{}, false
	}
	patch := 0
	if len(parts) >= serverVersionPatchPart {
		p, err := strconv.Atoi(readNumericPrefix(parts[2]))
		if err == nil {
			patch = p
		}
	}

	return pdnsVersion{major: major, minor: minor, patch: patch}, true
}

func readNumericPrefix(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return s[:i]
		}
	}
	return s
}

func isAtLeast(got, want pdnsVersion) bool {
	if got.major != want.major {
		return got.major > want.major
	}
	if got.minor != want.minor {
		return got.minor > want.minor
	}
	return got.patch >= want.patch
}

func decodeStats(response *http.Response, config *Config) error {
	stats, err := readLegacyStats(response.Body)
	if err != nil {
		return err
	}

	for i := range stats {
		if err := handleLegacyStat(response, stats[i], config); err != nil {
			return err
		}
	}
	return nil
}

func readLegacyStats(body io.Reader) ([]pdnsStat, error) {
	stats := make([]pdnsStat, 0)

	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &stats); err != nil {
		return nil, err
	}
	return stats, nil
}

func handleLegacyStat(response *http.Response, stat pdnsStat, config *Config) error {
	switch stat.Type {
	case "StatisticItem":
		return decodeStringStat(response, stat, config, false)
	case "MapStatisticItem":
		return decodeMapStatisticItem(stat, config)
	case "RingStatisticItem":
		return decodeRingStatisticItem(stat, config)
	default:
		// this allows for forward compatibility of powerdns adds new metrics types we just skip over them.
		// we emit a metric so that we know this is happening. powerdns.(recursor|authoritative).unknown.(type)
		return decodeStringStat(response, stat, config, true)
	}
}

func decodeMapStatisticItem(stat pdnsStat, config *Config) error {
	arr, ok := stat.Value.([]interface{})
	if !ok {
		return nil
	}
	for _, raw := range arr {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := m["name"].(string)
		if !ok || name == "" {
			continue
		}

		val, ok := readInt64MetricValue(m["value"])
		if !ok {
			continue
		}

		n := fmt.Sprintf("%s-%s", stat.Name, name)
		if _, ok := config.counterCumulativeValues[n]; !ok {
			config.counterCumulativeValues[n] = -1
		}
		config.StatsChan <- Statistic{Name: n, Type: counterCumulative, Value: val}
	}
	return nil
}

func decodeRingStatisticItem(stat pdnsStat, config *Config) error {
	str, ok := stat.Size.(string)
	if !ok {
		return nil
	}
	val, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return fmt.Errorf("unable to convert %s value string to int64 in decodeStats()", str)
	}
	if _, ok := config.counterCumulativeValues[stat.Name]; !ok {
		config.counterCumulativeValues[stat.Name] = -1
	}
	config.StatsChan <- Statistic{Name: stat.Name, Type: gauge, Value: val}
	return nil
}

func readInt64MetricValue(valRaw interface{}) (int64, bool) {
	switch v := valRaw.(type) {
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

func decodePrometheusStats(response *http.Response, config *Config) error {
	// expfmt's parser is strict and fails on malformed sample lines.
	// Historically this project skipped malformed lines (e.g., non-numeric values),
	// so we filter those out before parsing.
	emitHistograms := config != nil && config.histograms != nil && *config.histograms
	cleaned, err := cleanPrometheusBody(response.Body)
	if err != nil {
		return err
	}
	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(cleaned)
	if err != nil {
		return err
	}
	emitPrometheusFamilies(families, config, emitHistograms)
	return nil
}

func cleanPrometheusBody(body io.Reader) (io.Reader, error) {
	var cleaned strings.Builder
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			cleaned.WriteString(line)
			cleaned.WriteByte('\n')
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < prometheusSampleMinFields {
			continue
		}
		if _, err := strconv.ParseFloat(fields[1], 64); err != nil {
			continue
		}
		cleaned.WriteString(line)
		cleaned.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return strings.NewReader(cleaned.String()), nil
}

func uint64ToInt64Clamp(v uint64) int64 {
	max := ^uint64(0) >> 1
	if v > max {
		return int64(max)
	}
	return int64(v)
}

func emitPrometheusFamilies(families map[string]*dto.MetricFamily, config *Config, emitHistograms bool) {
	emit := func(name string, metricType string, val int64) {
		if metricType == counterCumulative {
			if _, ok := config.counterCumulativeValues[name]; !ok {
				config.counterCumulativeValues[name] = -1
			}
		}
		config.StatsChan <- Statistic{Name: name, Type: metricType, Value: val}
	}

	for _, family := range families {
		emitPrometheusFamily(family, emit, emitHistograms)
	}
}

func emitPrometheusFamily(
	family *dto.MetricFamily,
	emit func(name string, metricType string, val int64),
	emitHistograms bool,
) {
	if family == nil {
		return
	}
	name := family.GetName()
	if name == "" {
		return
	}

	familyType := family.GetType().String()
	sType := gauge
	if familyType == "COUNTER" {
		sType = counterCumulative
	}

	for _, m := range family.Metric {
		if m == nil {
			continue
		}

		switch familyType {
		case "COUNTER":
			if m.Counter == nil {
				continue
			}
			emit(name, sType, int64(m.GetCounter().GetValue()))
		case "GAUGE":
			if m.Gauge == nil {
				continue
			}
			emit(name, sType, int64(m.GetGauge().GetValue()))
		case "UNTYPED":
			if m.Untyped == nil {
				continue
			}
			emit(name, sType, int64(m.GetUntyped().GetValue()))
		case "HISTOGRAM":
			if !emitHistograms || m.Histogram == nil {
				continue
			}
			h := m.GetHistogram()
			emit(name+"_count", counterCumulative, uint64ToInt64Clamp(h.GetSampleCount()))
			emit(name+"_sum", counterCumulative, int64(h.GetSampleSum()))
		default:
			continue
		}
	}
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
