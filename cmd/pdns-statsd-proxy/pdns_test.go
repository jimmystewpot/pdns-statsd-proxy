package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type trackCloseBody struct {
	r      io.Reader
	closed *bool
}

func (b trackCloseBody) Read(p []byte) (int, error) { return b.r.Read(p) }

func (b trackCloseBody) Close() error {
	if b.closed != nil {
		*b.closed = true
	}
	return nil
}

func readpdnsTestData(version string) string {
	vers := strings.ReplaceAll(version, ".", "_")
	jsonFile := fmt.Sprintf("pdns_response_test_data/%s.json", vers)
	f, _ := os.ReadFile(jsonFile)

	return string(f)
}

func readpdnsPromTestData(filename string) string {
	f, _ := os.ReadFile(fmt.Sprintf("pdns_response_test_data/%s", filename))

	return string(f)
}

func TestReadNumericPrefix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "digits only", in: "123", want: "123"},
		{name: "digits then suffix", in: "42-test", want: "42"},
		{name: "no digits", in: "abc", want: ""},
		{name: "empty", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := readNumericPrefix(tt.in); got != tt.want {
				t.Fatalf("readNumericPrefix(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestReadInt64MetricValue(t *testing.T) {
	t.Run("string ok", func(t *testing.T) {
		got, ok := readInt64MetricValue("42")
		if !ok || got != 42 {
			t.Fatalf("readInt64MetricValue(\"42\") = (%d,%v), want (42,true)", got, ok)
		}
	})

	t.Run("string invalid", func(t *testing.T) {
		_, ok := readInt64MetricValue("nope")
		if ok {
			t.Fatalf("expected invalid string to return ok=false")
		}
	})

	t.Run("float64", func(t *testing.T) {
		got, ok := readInt64MetricValue(float64(3))
		if !ok || got != 3 {
			t.Fatalf("readInt64MetricValue(float64(3)) = (%d,%v), want (3,true)", got, ok)
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		_, ok := readInt64MetricValue(true)
		if ok {
			t.Fatalf("expected unsupported type to return ok=false")
		}
	})
}

func TestUint64ToInt64Clamp(t *testing.T) {
	maxInt64 := uint64(^uint64(0) >> 1)

	if got := uint64ToInt64Clamp(maxInt64); got != int64(maxInt64) {
		t.Fatalf("uint64ToInt64Clamp(max) = %d, want %d", got, int64(maxInt64))
	}

	if got := uint64ToInt64Clamp(maxInt64 + 1); got != int64(maxInt64) {
		t.Fatalf("uint64ToInt64Clamp(max+1) = %d, want %d", got, int64(maxInt64))
	}
}

func TestPoll_PostDiscovery_UsesLegacyPathWhenPrometheusDisabled(t *testing.T) {
	var gotURL string
	var gotAPIKey string
	var gotUserAgent string
	closed := false

	pdns := &pdnsClient{
		Client: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			gotURL = r.URL.String()
			gotAPIKey = r.Header.Get("X-API-Key")
			gotUserAgent = r.Header.Get("User-Agent")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       trackCloseBody{r: strings.NewReader("ok"), closed: &closed},
				Header:     make(http.Header),
				Request:    r,
			}, nil
		})},
		APIKey:              "test-key",
		legacyPath:          "http://example.local/legacy",
		prometheusPath:      "http://example.local/metrics",
		Host:                "http://example.local/legacy",
		serverVersionParsed: true,
		usePrometheus:       false,
	}

	resp, err := pdns.Poll()
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	defer resp.Body.Close()

	if gotURL != pdns.legacyPath {
		t.Fatalf("expected legacy URL %q, got %q", pdns.legacyPath, gotURL)
	}
	if gotAPIKey != "test-key" {
		t.Fatalf("expected X-API-Key %q, got %q", "test-key", gotAPIKey)
	}
	if gotUserAgent != provider {
		t.Fatalf("expected User-Agent %q, got %q", provider, gotUserAgent)
	}
	if closed {
		t.Fatalf("did not expect response body to be closed by Poll() on success")
	}
}

func TestPoll_PostDiscovery_UsesPrometheusPathWhenEnabled(t *testing.T) {
	config := testConfig()
	_ = config

	var gotURL string
	closed := false

	pdns := &pdnsClient{
		Client: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			gotURL = r.URL.String()
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       trackCloseBody{r: strings.NewReader("ok"), closed: &closed},
				Header:     make(http.Header),
				Request:    r,
			}, nil
		})},
		APIKey:              "test-key",
		legacyPath:          "http://example.local/legacy",
		prometheusPath:      "http://example.local/metrics",
		Host:                "http://example.local/metrics",
		serverVersionParsed: true,
		usePrometheus:       true,
	}

	resp, err := pdns.Poll()
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	defer resp.Body.Close()

	if gotURL != pdns.prometheusPath {
		t.Fatalf("expected prometheus URL %q, got %q", pdns.prometheusPath, gotURL)
	}
	if closed {
		t.Fatalf("did not expect response body to be closed by Poll() on success")
	}
}

func TestPoll_Non200_ClosesBodyAndReturnsError(t *testing.T) {
	closed := false

	pdns := &pdnsClient{
		Client: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       trackCloseBody{r: strings.NewReader("nope"), closed: &closed},
				Header:     make(http.Header),
				Request:    r,
			}, nil
		})},
		APIKey:              "test-key",
		legacyPath:          "http://example.local/legacy",
		prometheusPath:      "http://example.local/metrics",
		Host:                "http://example.local/legacy",
		serverVersionParsed: true,
		usePrometheus:       false,
	}

	_, err := pdns.Poll()
	if err == nil {
		t.Fatalf("expected Poll() to return error")
	}
	if !closed {
		t.Fatalf("expected response body to be closed on non-200 status")
	}
}

func TestDecodePrometheusStats_Fixture_Auth440(t *testing.T) {
	config := testConfig()
	config.histograms = boolPtr(false)

	body := readpdnsPromTestData("auth-4_4_0_prometheus.prom")
	if body == "" {
		t.Fatalf("expected auth-4_4_0_prometheus.prom fixture to be readable")
	}

	resp := &http.Response{
		Body:   io.NopCloser(strings.NewReader(body)),
		Header: make(http.Header),
	}

	if err := decodePrometheusStats(resp, config); err != nil {
		t.Fatalf("decodePrometheusStats() error = %v", err)
	}

	got := make(map[string]Statistic)
	for len(config.StatsChan) > 0 {
		s := <-config.StatsChan
		got[s.Name] = s
	}

	// Validate at least one known gauge and one known counter from the fixture.
	fd, ok := got["pdns_auth_fd_usage"]
	if !ok {
		t.Fatalf("expected metric pdns_auth_fd_usage")
	}
	if fd.Type != gauge {
		t.Fatalf("pdns_auth_fd_usage = %+v, want type=%s", fd, gauge)
	}

	cpu, ok := got["pdns_auth_cpu_iowait"]
	if !ok {
		t.Fatalf("expected metric pdns_auth_cpu_iowait")
	}
	if cpu.Type != counterCumulative {
		t.Fatalf("pdns_auth_cpu_iowait = %+v, want type=%s", cpu, counterCumulative)
	}
	if _, ok := config.counterCumulativeValues["pdns_auth_cpu_iowait"]; !ok {
		t.Fatalf("expected counterCumulativeValues to contain pdns_auth_cpu_iowait")
	}
}

func TestDecodePrometheusStats_HistogramsGated(t *testing.T) {
	base := strings.Join([]string{
		"# HELP my_hist Example histogram",
		"# TYPE my_hist histogram",
		"my_hist_bucket{le=\"1\"} 0",
		"my_hist_bucket{le=\"+Inf\"} 2",
		"my_hist_sum 3",
		"my_hist_count 2",
	}, "\n")

	{ // default: histograms disabled
		config := testConfig()
		config.histograms = boolPtr(false)
		resp := &http.Response{Body: io.NopCloser(strings.NewReader(base)), Header: make(http.Header)}
		if err := decodePrometheusStats(resp, config); err != nil {
			t.Fatalf("decodePrometheusStats() error = %v", err)
		}
		for len(config.StatsChan) > 0 {
			s := <-config.StatsChan
			if s.Name == "my_hist_count" || s.Name == "my_hist_sum" {
				t.Fatalf("did not expect histogram metric %s to be emitted when histograms=false", s.Name)
			}
		}
	}

	{ // histograms enabled
		config := testConfig()
		config.histograms = boolPtr(true)
		resp := &http.Response{Body: io.NopCloser(strings.NewReader(base)), Header: make(http.Header)}
		if err := decodePrometheusStats(resp, config); err != nil {
			t.Fatalf("decodePrometheusStats() error = %v", err)
		}

		seenCount := false
		seenSum := false
		for len(config.StatsChan) > 0 {
			s := <-config.StatsChan
			switch s.Name {
			case "my_hist_count":
				seenCount = true
			case "my_hist_sum":
				seenSum = true
			}
		}
		if !seenCount || !seenSum {
			t.Fatalf("expected histogram metrics to be emitted when histograms=true (count=%v sum=%v)", seenCount, seenSum)
		}
	}
}

func TestPoll_Discovery_SwitchesToPrometheus(t *testing.T) {
	config := testConfig()

	var legacyHits, promHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/servers/localhost/statistics":
			legacyHits++
			w.Header().Set("Server", "PowerDNS/4.9.0")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "[]")
		case "/metrics":
			promHits++
			w.Header().Set("Server", "PowerDNS/4.9.0")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "# TYPE foo counter\nfoo 1\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	hostPort := strings.TrimPrefix(srv.URL, "http://")
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		t.Fatalf("SplitHostPort(%q) error = %v", hostPort, err)
	}
	config.pdnsHost = stringPtr(host)
	config.pdnsPort = stringPtr(port)
	config.pdnsAPIKey = stringPtr("x")

	pdns := new(pdnsClient)
	pdns.Initialise(config)

	resp, err := pdns.Poll()
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if resp == nil {
		t.Fatalf("expected response")
	}
	defer resp.Body.Close()

	if !pdns.usePrometheus {
		t.Fatalf("expected usePrometheus=true")
	}
	if legacyHits != 1 || promHits != 1 {
		t.Fatalf("expected legacyHits=1 promHits=1 got legacyHits=%d promHits=%d", legacyHits, promHits)
	}
}

func TestPoll_Discovery_StaysLegacy(t *testing.T) {
	config := testConfig()

	var legacyHits, promHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/servers/localhost/statistics":
			legacyHits++
			w.Header().Set("Server", "PowerDNS/4.2.0")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "[]")
		case "/metrics":
			promHits++
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "foo 1\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	hostPort := strings.TrimPrefix(srv.URL, "http://")
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		t.Fatalf("SplitHostPort(%q) error = %v", hostPort, err)
	}
	config.pdnsHost = stringPtr(host)
	config.pdnsPort = stringPtr(port)
	config.pdnsAPIKey = stringPtr("x")

	pdns := new(pdnsClient)
	pdns.Initialise(config)

	resp, err := pdns.Poll()
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	defer resp.Body.Close()

	if pdns.usePrometheus {
		t.Fatalf("expected usePrometheus=false")
	}
	if legacyHits != 1 {
		t.Fatalf("expected legacyHits=1 got %d", legacyHits)
	}
	if promHits != 0 {
		t.Fatalf("expected promHits=0 got %d", promHits)
	}
}

func TestPoll_ErrorOnNon200(t *testing.T) {
	config := testConfig()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "PowerDNS/4.9.0")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "boom")
	}))
	defer srv.Close()

	hostPort := strings.TrimPrefix(srv.URL, "http://")
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		t.Fatalf("SplitHostPort(%q) error = %v", hostPort, err)
	}
	config.pdnsHost = stringPtr(host)
	config.pdnsPort = stringPtr(port)
	config.pdnsAPIKey = stringPtr("x")

	pdns := new(pdnsClient)
	pdns.Initialise(config)

	resp, err := pdns.Poll()
	if err == nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		t.Fatalf("expected error")
	}

	if resp != nil && resp.StatusCode != 0 {
		_ = strconv.ErrRange
	}
}

func testDNSClient(config *Config) *pdnsClient {
	// initiate the powerdns client.
	pdnsClient := new(pdnsClient)
	pdnsClient.Initialise(config)
	return pdnsClient
}

func testHeader() http.Header {
	header := make(http.Header)
	header.Set("Server", "PowerDNS/4.9.0")
	return header
}

func TestDecodeStats(t *testing.T) {
	type args struct {
		response *http.Response
		config   *Config
	}
	tests := []struct {
		name     string
		args     args
		count    int
		recursor bool
		wantErr  bool
	}{
		{
			name: "recursor 4.3 valid",
			args: args{
				response: &http.Response{
					Body:   io.NopCloser(strings.NewReader(readpdnsTestData("recursor-4.3.3"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			count:    114,
			recursor: true,
			wantErr:  false,
		},
		{
			name: "recursor 4.3 invalid",
			args: args{
				response: &http.Response{
					Body:   io.NopCloser(strings.NewReader(readpdnsTestData("recursor-4.3.3-bad"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			count:    114,
			recursor: true,
			wantErr:  true,
		},
		{
			name: "auth 4.3 valid",
			args: args{
				response: &http.Response{
					Body:   io.NopCloser(strings.NewReader(readpdnsTestData("auth-4.3.0"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			count:    86,
			recursor: false,
			wantErr:  false,
		},
		{
			name: "auth 4.3 invalid",
			args: args{
				response: &http.Response{
					Body:   io.NopCloser(strings.NewReader(readpdnsTestData("auth-4.3.0-bad"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			count:    78,
			recursor: false,
			wantErr:  true,
		},
		{
			name: "recursor unknown metric type",
			args: args{
				response: &http.Response{
					Body:   io.NopCloser(strings.NewReader(readpdnsTestData("recursor-unknown"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			count:    115,
			recursor: true,
			wantErr:  false,
		},
		{
			name: "recursor map type invalid int",
			args: args{
				response: &http.Response{
					Body:   io.NopCloser(strings.NewReader(readpdnsTestData("recursor-4.3.3-bad_ints"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			count:    113,
			recursor: true,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.recursor {
				tt.args.config.recursor = ptrBool(tt.recursor)
			}
			if err := decodeStats(tt.args.response, tt.args.config); (err != nil) != tt.wantErr {
				t.Errorf("decodeStats() error = %v, wantErr %v", err, tt.wantErr)
			}
			if (len(tt.args.config.StatsChan) != tt.count) != tt.wantErr {
				t.Errorf("expected %d stats got %d", tt.count, len(tt.args.config.StatsChan))
			}
		})
	}
}

func TestPdnsClientWorker(t *testing.T) {
	type args struct {
		config *Config
		err    bool
	}
	tests := []struct {
		name              string
		args              args
		testDataFile      string
		testResponseCode  int
		testAuthoritative bool
	}{
		{
			name: "Good HTTP Response, Good Payload",
			args: args{
				config: testConfig(),
			},
			testDataFile:     "recursor-4.3.3",
			testResponseCode: http.StatusOK,
		},
		{
			name: "Good HTTP Response, Bad Payload",
			args: args{
				config: testConfig(),
			},
			testDataFile:     "recursor-4.3.3-bad",
			testResponseCode: http.StatusOK,
		},
		{
			name: "Bad HTTP Response, Good Payload",
			args: args{
				config: testConfig(),
			},
			testDataFile:     "recursor-4.3.3",
			testResponseCode: http.StatusUnauthorized,
		},
		{
			name: "Bad HTTP Response, Bad Payload",
			args: args{
				config: testConfig(),
			},
			testDataFile:     "recursor-4.3.3-bad",
			testResponseCode: http.StatusUnauthorized,
		},
		{
			name: "Good HTTP response, Good Payload Unknown Entry",
			args: args{
				config: testConfig(),
			},
			testDataFile:     "recursor-unknown",
			testResponseCode: http.StatusOK,
		},
		{
			name: "Good HTTP response, Bad Payload Unknown Entry",
			args: args{
				config: testConfig(),
			},
			testDataFile:     "recursor-unknown-bad",
			testResponseCode: http.StatusOK,
		},
		{
			name: "Good HTTP response, Bad Server",
			args: args{
				config: testConfig(),
				err:    true,
			},
			testDataFile:     "recursor-unknown-bad",
			testResponseCode: http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdns := testDNSClient(tt.args.config)

			// setup a local http mock to simulate the powerdns api
			var listener net.Listener
			var err error
			switch tt.args.err {
			case true:
				listener, err = net.Listen("tcp", net.JoinHostPort(*tt.args.config.pdnsHost, "5555"))
			case false:
				listener, err = net.Listen("tcp", net.JoinHostPort(*tt.args.config.pdnsHost, *tt.args.config.pdnsPort))
			}
			if err != nil {
				t.Errorf("got error trying to start mock http listener: %s", err)
			}

			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.testResponseCode)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Server", "PowerDNS/4.0.0-test")
				fmt.Fprint(w, readpdnsTestData(tt.testDataFile))
			}))
			srv.Listener = listener
			srv.Start()

			// close the channel in the background to test a correct exit state.
			go func(config *Config) {
				time.Sleep(time.Duration(1000) * time.Millisecond)
				close(config.stop)
			}(tt.args.config)

			go pdns.Worker(tt.args.config)
			time.Sleep(time.Duration(1500) * time.Millisecond)
			<-tt.args.config.pdnsExited

			// close the mock server.
			srv.Close()
		})
	}
}

func Test_pdnsClient_Poll(t *testing.T) {
	type fields struct {
		Host   string
		APIKey string
		Client *http.Client
	}
	tests := []struct {
		name    string
		fields  fields
		want    *http.Response
		wantErr bool
	}{
		{
			name: "Known bad configuration",
			fields: fields{
				Host:   "\1232\\1\\11",
				APIKey: "bad-entry",
				Client: &http.Client{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdns := &pdnsClient{
				Host:   tt.fields.Host,
				APIKey: tt.fields.APIKey,
				Client: tt.fields.Client,
			}
			//nolint:bodyclose // this is testing the error handling working Polls are handled elsewhere in tests.
			_, err := pdns.Poll()
			if (err != nil) != tt.wantErr {
				t.Errorf("pdnsClient.Poll() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func ptrBool(b bool) *bool {
	return &b
}

func TestParsePDNSServerHeader(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		want      pdnsVersion
		wantFound bool
	}{
		{
			name:      "plain semantic version",
			header:    "PowerDNS/4.3.0",
			want:      pdnsVersion{major: 4, minor: 3, patch: 0},
			wantFound: true,
		},
		{
			name:      "version with suffix",
			header:    "PowerDNS/4.3.0-test",
			want:      pdnsVersion{major: 4, minor: 3, patch: 0},
			wantFound: true,
		},
		{
			name:      "header contains PowerDNS token",
			header:    "nginx/1.21.6 (some) PowerDNS/4.2.1",
			want:      pdnsVersion{major: 4, minor: 2, patch: 1},
			wantFound: true,
		},
		{
			name:      "missing version",
			header:    "PowerDNS/",
			wantFound: false,
		},
		{
			name:      "no PowerDNS token",
			header:    "Apache/2.4.0",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parsePDNSServerHeader(tt.header)
			if ok != tt.wantFound {
				t.Fatalf("parsePDNSServerHeader() ok = %v, want %v", ok, tt.wantFound)
			}
			if !tt.wantFound {
				return
			}
			if got != tt.want {
				t.Fatalf("parsePDNSServerHeader() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestIsAtLeast(t *testing.T) {
	tests := []struct {
		name string
		got  pdnsVersion
		want pdnsVersion
		ok   bool
	}{
		{
			name: "equal",
			got:  pdnsVersion{major: 4, minor: 3, patch: 0},
			want: pdnsVersion{major: 4, minor: 3, patch: 0},
			ok:   true,
		},
		{
			name: "newer minor",
			got:  pdnsVersion{major: 4, minor: 4, patch: 0},
			want: pdnsVersion{major: 4, minor: 3, patch: 0},
			ok:   true,
		},
		{
			name: "older minor",
			got:  pdnsVersion{major: 4, minor: 2, patch: 99},
			want: pdnsVersion{major: 4, minor: 3, patch: 0},
			ok:   false,
		},
		{
			name: "older patch",
			got:  pdnsVersion{major: 4, minor: 3, patch: 0},
			want: pdnsVersion{major: 4, minor: 3, patch: 1},
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAtLeast(tt.got, tt.want); got != tt.ok {
				t.Fatalf("isAtLeast(%+v, %+v) = %v, want %v", tt.got, tt.want, got, tt.ok)
			}
		})
	}
}

func TestDecodePrometheusStats(t *testing.T) {
	config := testConfig()

	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"# HELP pdns_recursor_all_outqueries Number of outgoing UDP queries since starting",
			"# TYPE pdns_recursor_all_outqueries counter",
			"pdns_recursor_all_outqueries 20",
			"# TYPE pdns_recursor_uptime gauge",
			"pdns_recursor_uptime{foo=\"bar\"} 142",
			"pdns_recursor_no_type 3",
			"pdns_recursor_bad abc",
		}, "\n"))),
		Header: make(http.Header),
	}

	if err := decodePrometheusStats(resp, config); err != nil {
		t.Fatalf("decodePrometheusStats() error = %v", err)
	}

	got := make(map[string]Statistic)
	for len(config.StatsChan) > 0 {
		s := <-config.StatsChan
		got[s.Name] = s
	}

	all, ok := got["pdns_recursor_all_outqueries"]
	if !ok {
		t.Fatalf("expected metric pdns_recursor_all_outqueries")
	}
	if all.Type != counterCumulative || all.Value != 20 {
		t.Fatalf("pdns_recursor_all_outqueries = %+v, want type=%s value=%d", all, counterCumulative, 20)
	}
	if _, ok := config.counterCumulativeValues["pdns_recursor_all_outqueries"]; !ok {
		t.Fatalf("expected counterCumulativeValues to contain pdns_recursor_all_outqueries")
	}

	up, ok := got["pdns_recursor_uptime"]
	if !ok {
		t.Fatalf("expected metric pdns_recursor_uptime")
	}
	if up.Type != gauge || up.Value != 142 {
		t.Fatalf("pdns_recursor_uptime = %+v, want type=%s value=%d", up, gauge, 142)
	}

	nt, ok := got["pdns_recursor_no_type"]
	if !ok {
		t.Fatalf("expected metric pdns_recursor_no_type")
	}
	if nt.Type != gauge || nt.Value != 3 {
		t.Fatalf("pdns_recursor_no_type = %+v, want type=%s value=%d", nt, gauge, 3)
	}

	if _, ok := got["pdns_recursor_bad"]; ok {
		t.Fatalf("did not expect pdns_recursor_bad to be emitted")
	}
}
