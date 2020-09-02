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

	"go.uber.org/zap"
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
	err := pdnsClient.Initialise(config)
	if err != nil {
		log.Fatal("unable to initialise powerdns client",
			zap.Error(err),
		)
	}
	return pdnsClient
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
					Body: ioutil.NopCloser(strings.NewReader(readpdnsTestData("recursor-4.3.3"))),
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
					Body: ioutil.NopCloser(strings.NewReader(readpdnsTestData("recursor-4.3.3-bad"))),
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
					Body: ioutil.NopCloser(strings.NewReader(readpdnsTestData("auth-4.3.0"))),
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
					Body: ioutil.NopCloser(strings.NewReader(readpdnsTestData("auth-4.3.0-bad"))),
				},
				config: testConfig(),
			},
			count:    78,
			recursor: false,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counterCumulativeValues = make(map[string]int64)
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
		name             string
		args             args
		testDataFile     string
		testResponseCode int
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdns := testDNSClient(tt.args.config)

			// setup a local http mock to simulate the powerdns api
			listener, err := net.Listen("tcp", "127.0.0.1:8089")
			if err != nil {
				t.Errorf("got error trying to start mock listener: %s", err)
			}
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.testResponseCode)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, readpdnsTestData(tt.testDataFile))
			}))
			srv.Listener = listener
			srv.Start()

			// close the channel in the background to test a correct exit state.
			go func(config *Config) {
				time.Sleep(time.Duration(1000) * time.Millisecond)
				close(config.Done)
			}(tt.args.config)

			go pdns.Worker(tt.args.config)
			time.Sleep(time.Duration(1500) * time.Millisecond)

			// close the mock server.
			srv.Close()
		})
	}
}
