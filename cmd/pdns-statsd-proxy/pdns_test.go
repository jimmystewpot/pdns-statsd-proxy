package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func readpdnsTestData(version string) string {
	vers := strings.ReplaceAll(version, ".", "_")
	jsonFile := fmt.Sprintf("pdns_response_test_data/%s.json", vers)
	f, _ := os.ReadFile(jsonFile)

	return string(f)
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
