package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func readpdnsTestData(version string) string {
	vers := strings.ReplaceAll(version, ".", "_")
	jsonFile := fmt.Sprintf("pdns_response_test_data/%s.json", vers)
	f, _ := ioutil.ReadFile(jsonFile)

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

func Test_decodeStats(t *testing.T) {
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
					Body:   ioutil.NopCloser(strings.NewReader(readpdnsTestData("recursor-4.3.3"))),
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
					Body:   ioutil.NopCloser(strings.NewReader(readpdnsTestData("recursor-4.3.3-bad"))),
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
					Body:   ioutil.NopCloser(strings.NewReader(readpdnsTestData("auth-4.3.0"))),
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
					Body:   ioutil.NopCloser(strings.NewReader(readpdnsTestData("auth-4.3.0-bad"))),
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
					Body:   ioutil.NopCloser(strings.NewReader(readpdnsTestData("recursor-unknown"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			count:    115,
			recursor: true,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.recursor {
				tt.args.config.recursor = &tt.recursor
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

func Test_pdnsClient_Worker(t *testing.T) {
	type args struct {
		config *Config
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdns := testDNSClient(tt.args.config)

			// setup a local http mock to simulate the powerdns api
			listener, err := net.Listen("tcp", net.JoinHostPort(*tt.args.config.pdnsHost, *tt.args.config.pdnsPort))
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
				config.pdnsDone <- true
			}(tt.args.config)

			go pdns.Worker(tt.args.config)
			time.Sleep(time.Duration(1500) * time.Millisecond)

			// close the mock server.
			srv.Close()
		})
	}
}
