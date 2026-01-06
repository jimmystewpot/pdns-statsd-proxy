package main

import (
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

const udpReadBufSize = 65535

type errStatsClient struct{}

func (errStatsClient) Gauge(string, float64, []string, float64) error { return io.ErrClosedPipe }
func (errStatsClient) Count(string, int64, []string, float64) error   { return io.ErrClosedPipe }
func (errStatsClient) Close() error                                   { return nil }

type closeErrStatsClient struct{}

func (closeErrStatsClient) Gauge(string, float64, []string, float64) error { return nil }
func (closeErrStatsClient) Count(string, int64, []string, float64) error   { return nil }
func (closeErrStatsClient) Close() error                                   { return io.ErrClosedPipe }

type recordStatsClient struct {
	countCalls []struct {
		name  string
		value int64
	}
}

func (r *recordStatsClient) Gauge(string, float64, []string, float64) error { return nil }
func (r *recordStatsClient) Count(name string, value int64, tags []string, rate float64) error {
	r.countCalls = append(r.countCalls, struct {
		name  string
		value int64
	}{name: name, value: value})
	return nil
}
func (r *recordStatsClient) Close() error { return nil }

func TestGaugeMetrics(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{
			name:    "valid gauge",
			want:    "uptime",
			wantErr: false,
		},
		{
			name:    "invalid gauge",
			want:    "xxxx",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gaugeMetrics()
			if _, ok := got[tt.want]; !ok {
				if !tt.wantErr {
					t.Errorf("gaugeMetrics() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestStatsWorker_ExitsWhenStatsChanClosed(t *testing.T) {
	config := testConfig()
	stats = errStatsClient{}

	go statsWorker(config)
	close(config.StatsChan)

	select {
	case <-config.statsExited:
		return
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for stats worker to exit")
	}
}

func TestStatsWorker_StopsOnStopChannelAndAttemptsClose(t *testing.T) {
	config := testConfig()
	stats = closeErrStatsClient{}

	go statsWorker(config)
	close(config.stop)

	select {
	case <-config.statsExited:
		return
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for stats worker to exit")
	}
}

func Test_processStats_CounterCumulative_SkipFirstThenEmitDelta(t *testing.T) {
	recorder := &recordStatsClient{}
	stats = recorder

	counters := map[string]int64{"foo": -1}

	// First value should not emit a delta (spike avoidance), but should update stored value.
	if err := processStats(Statistic{Name: "foo", Type: counterCumulative, Value: 10}, counters); err != nil {
		t.Fatalf("processStats() err = %v", err)
	}
	if counters["foo"] != 10 {
		t.Fatalf("expected stored counter value to be updated to 10, got %d", counters["foo"])
	}
	if len(recorder.countCalls) != 0 {
		t.Fatalf("did not expect Count to be called on first value")
	}

	// Second value should emit delta (15 - 10 = 5).
	if err := processStats(Statistic{Name: "foo", Type: counterCumulative, Value: 15}, counters); err != nil {
		t.Fatalf("processStats() err = %v", err)
	}
	if len(recorder.countCalls) != 1 {
		t.Fatalf("expected 1 Count call, got %d", len(recorder.countCalls))
	}
	if recorder.countCalls[0].name != "foo" || recorder.countCalls[0].value != 5 {
		t.Fatalf("unexpected Count call: %+v", recorder.countCalls[0])
	}
}

func TestStatsWorker(t *testing.T) {
	var wg sync.WaitGroup
	config := testConfig()

	type args struct {
		config *Config
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "stats worker start",
			args: args{
				config: config,
			},
		},
	}
	// setup a backgroun udp listener to accept the statsd datagrams/packets.
	go func(config *Config) {
		udpAddr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(*config.statsHost, *config.statsPort))
		if err != nil {
			t.Error(err)
		}
		statsSrv, err := net.ListenUDP("udp4", udpAddr)
		if err != nil {
			t.Error(err)
		}
		defer statsSrv.Close()
		for {
			select {
			case <-config.stop:
				return
			default:
				buf := make([]byte, udpReadBufSize)
				_, _, err := statsSrv.ReadFromUDP(buf)
				if err != nil {
					continue
				}
			}
		}
	}(config)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats, _ = NewStatsClient(tt.args.config)
			// this function sends the metrics down the channel for the worker to process.
			// it needs to have a wait group to wait for the system to prime otherwise it causes
			// a data race.
			wg.Add(1)
			go func(config *Config, wg *sync.WaitGroup) {
				for r := 0; r <= 3; r++ {
					responseBody := &http.Response{
						Body: io.NopCloser(strings.NewReader(readpdnsTestData("recursor-4.3.3"))),
					}
					err := decodeStats(responseBody, config)
					if err != nil {
						t.Error(err)
					}
					responseBody.Body.Close()
				}
				wg.Done()
			}(tt.args.config, &wg)

			// wait until the statistics have been sent.
			wg.Wait()
			go statsWorker(tt.args.config)

			time.Sleep(time.Duration(3500) * time.Millisecond)
			close(config.stop)
			<-tt.args.config.statsExited
		})
	}
}

func TestNewStatsClient(t *testing.T) {
	type args struct {
		config *Config
	}
	tests := []struct {
		name      string
		args      args
		statsHost string
		wantErr   bool
	}{
		{
			name: "Good Configuration",
			args: args{
				config: testConfig(),
			},
			statsHost: "127.0.0.1",
			wantErr:   false,
		},
		{
			name: "Bad Configuration - invalid host",
			args: args{
				config: testConfig(),
			},
			statsHost: "",
			wantErr:   true,
		},
		{
			name: "Bad Configuration - invalid ip",
			args: args{
				config: testConfig(),
			},
			statsHost: "a.b.c.d",
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.config.statsHost = stringPtr(tt.statsHost)
			_, err := NewStatsClient(tt.args.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewStatsClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_processStats(t *testing.T) {
	type args struct {
		s                       Statistic
		counterCumulativeValues map[string]int64
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Gauge Metric",
			args: args{
				s: Statistic{
					Name:  "Foo",
					Type:  gauge,
					Value: 1,
				},
				counterCumulativeValues: map[string]int64{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats = errStatsClient{}
			if err := processStats(tt.args.s, tt.args.counterCumulativeValues); (err != nil) != tt.wantErr {
				t.Errorf("processStats() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_zeroMin(t *testing.T) {
	type args struct {
		x int64
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "less than zero",
			args: args{
				x: -1,
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := zeroMin(tt.args.x); got != tt.want {
				t.Errorf("zeroMin() = %v, want %v", got, tt.want)
			}
		})
	}
}
