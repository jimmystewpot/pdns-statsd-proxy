package main

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"
)

const (
	localhost string = "127.0.0.1"
)

func testConfig() *Config {
	debug := getEnvStr("DEBUG", "")

	// for testing we need to set if this global variable is already set
	// so we don't have a race condition.
	if reflect.ValueOf(log).IsNil() {
		if *debug == "" {
			l, _ := zap.NewProduction()
			log = l.Named(provider)
		} else {
			log = zap.NewExample(zap.AddCaller(), zap.WithCaller(true)).Named(provider)
		}
	}

	return &Config{
		statsHost:               stringPtr(localhost),
		statsPort:               stringPtr("8199"),
		interval:                timePtr(time.Duration(1) * time.Second),
		pdnsHost:                stringPtr(localhost),
		pdnsPort:                stringPtr("9999"),
		pdnsAPIKey:              stringPtr("x-api-key"),
		recursor:                boolPtr(true),
		histograms:              boolPtr(false),
		counterCumulativeValues: make(map[string]int64),
		StatsChan:               make(chan Statistic, 1000),
		stop:                    make(chan struct{}),
		pdnsExited:              make(chan struct{}),
		statsExited:             make(chan struct{}),
	}
}

func TestApplyYAMLConfig_InvalidInterval(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	configFile = flag.String("config", "/etc/pdns-statsd-proxy/config.yaml", "Path to YAML config file")
	statsHost = flag.String("statsHost", "127.0.0.1", "The statsd server to emit metrics")
	statsPort = flag.String("statsPort", "8125", "The port that statsd is listening on")
	pdnsHost = flag.String("pdnsHost", "127.0.0.1", "The host to query for powerdns statistics")
	pdnsPort = flag.String("pdnsPort", "8080", "The port that PowerDNS API is accepting connections")
	pdnsAPIKey = flag.String("key", "", "The api key for the powerdns api")
	recursor = flag.Bool("recursor", true, "Query recursor statistics")
	histograms = flag.Bool("histograms", false, "")
	interval = flag.Duration("interval", defaultInterval, "")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("interval: not-a-duration\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() err = %v", err)
	}

	if err := applyYAMLConfig(cfgPath, map[string]bool{}); err == nil {
		t.Fatalf("expected applyYAMLConfig to return error for invalid interval")
	}
}

func TestApplyYAMLConfig_IntervalSecondsAndFlagOverride(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	configFile = flag.String("config", "/etc/pdns-statsd-proxy/config.yaml", "Path to YAML config file")
	statsHost = flag.String("statsHost", "127.0.0.1", "The statsd server to emit metrics")
	statsPort = flag.String("statsPort", "8125", "The port that statsd is listening on")
	pdnsHost = flag.String("pdnsHost", "127.0.0.1", "The host to query for powerdns statistics")
	pdnsPort = flag.String("pdnsPort", "8080", "The port that PowerDNS API is accepting connections")
	pdnsAPIKey = flag.String("key", "", "The api key for the powerdns api")
	recursor = flag.Bool("recursor", true, "Query recursor statistics")
	histograms = flag.Bool("histograms", false, "")
	interval = flag.Duration("interval", defaultInterval, "")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	contents := "statsHost: 10.0.0.1\ninterval: '30'\n"
	if err := os.WriteFile(cfgPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile() err = %v", err)
	}

	setFlags := map[string]bool{"statsHost": true}
	if err := applyYAMLConfig(cfgPath, setFlags); err != nil {
		t.Fatalf("applyYAMLConfig() err = %v", err)
	}

	if *statsHost != "127.0.0.1" {
		t.Fatalf("expected statsHost to be unchanged due to flag override, got %q", *statsHost)
	}
	if *interval != 30*time.Second {
		t.Fatalf("expected interval to be 30s, got %s", interval.String())
	}
}

func TestGetEnvStr(t *testing.T) {
	type args struct {
		name string
		def  string
	}
	tests := []struct {
		name string
		args args
		want string
		set  bool
	}{
		{
			name: "test environment FOO_BAR",
			args: args{
				name: "FOO_BAR",
				def:  "FOO",
			},
			want: "FOO",
			set:  false,
		},
		{
			name: "test environment BAR_FOO",
			args: args{
				name: "BAR_FOO",
				def:  "",
			},
			want: "DEFAULT",
			set:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				os.Setenv(tt.args.name, tt.want)
			}
			if got := getEnvStr(tt.args.name, tt.args.def); *got != tt.want {
				t.Errorf("getEnvStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

// timePtr returns a pointer for Time.Duration.
func timePtr(t time.Duration) *time.Duration {
	return &t
}

// stringPtr returns a pointer for an input string
func stringPtr(s string) *string {
	return &s
}

// boolPtr returns a pointer for an input boolean
func boolPtr(b bool) *bool {
	return &b
}

func TestConfigValidate(t *testing.T) {
	log = zap.NewExample(zap.AddCaller(), zap.WithCaller(true)).Named(provider)
	type fields struct {
		statsHost               *string
		statsPort               *string
		interval                *time.Duration
		pdnsHost                *string
		pdnsPort                *string
		pdnsAPIKey              *string
		recursor                *bool
		counterCumulativeValues map[string]int64
		StatsChan               chan Statistic
		stop                    chan struct{}
		pdnsExited              chan struct{}
		statsExited             chan struct{}
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "no API key",
			fields: fields{
				pdnsAPIKey: stringPtr(""),
				statsHost:  stringPtr("1.1.1.1"),
			},
			want: false,
		},
		{
			name: "no statsd host",
			fields: fields{
				pdnsAPIKey: stringPtr("API-KEY"),
				statsHost:  stringPtr(""),
			},
			want: false,
		},
		{
			name: "nil statsd host",
			fields: fields{
				pdnsAPIKey: stringPtr("API-KEY"),
				statsHost:  nil,
			},
			want: true,
		},
		{
			name: "nil api key",
			fields: fields{
				pdnsAPIKey: nil,
				statsHost:  stringPtr("127.0.0.1"),
			},
			want: false,
		},
		{
			name: "valid configuration",
			fields: fields{
				pdnsAPIKey: stringPtr("API-KEY"),
				statsHost:  stringPtr("Foo"),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				statsHost:               tt.fields.statsHost,
				statsPort:               tt.fields.statsPort,
				interval:                tt.fields.interval,
				pdnsHost:                tt.fields.pdnsHost,
				pdnsPort:                tt.fields.pdnsPort,
				pdnsAPIKey:              tt.fields.pdnsAPIKey,
				recursor:                tt.fields.recursor,
				counterCumulativeValues: tt.fields.counterCumulativeValues,
				StatsChan:               tt.fields.StatsChan,
				stop:                    tt.fields.stop,
				pdnsExited:              tt.fields.pdnsExited,
				statsExited:             tt.fields.statsExited,
			}
			if got := c.Validate(); got != tt.want {
				t.Errorf("Config.Validate() = %v, want %v", got, tt.want)
			}
		})
	}
}
