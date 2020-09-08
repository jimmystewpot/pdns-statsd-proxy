package main

import (
	"os"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"
)

func testConfig() *Config {
	// configuration is all okay, initialise the maps
	counterCumulativeValues = make(map[string]int64)
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
		statsHost:  stringPtr("127.0.0.1"),
		statsPort:  stringPtr("8199"),
		interval:   timePtr(time.Duration(1) * time.Second),
		pdnsHost:   stringPtr("127.0.0.1"),
		pdnsPort:   stringPtr("8089"),
		pdnsAPIKey: stringPtr("x-api-key"),
		recursor:   boolPtr(true),
		StatsChan:  make(chan Statistic, 1000),
		Done:       make(chan bool, 1),
		pdnsDone:   make(chan bool, 1),
		statsDone:  make(chan bool, 1),
	}
}

func Test_getEnvStr(t *testing.T) {
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

func Test_checkpdnsAPIKey(t *testing.T) {
	type args struct {
		config *Config
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Valid API Key",
			args: args{
				config: testConfig(),
			},
			want: true,
		},
		{
			name: "No API Key",
			args: args{
				config: &Config{
					pdnsAPIKey: stringPtr(""),
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := checkpdnsAPIKey(tt.args.config); got != tt.want {
				t.Errorf("checkpdnsAPIKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_checkStatsHost(t *testing.T) {
	type args struct {
		config *Config
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Valid Statsd Host",
			args: args{
				config: testConfig(),
			},
			want: true,
		},
		{
			name: "No Statsd host",
			args: args{
				config: &Config{
					statsHost: stringPtr(""),
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := checkStatsHost(tt.args.config); got != tt.want {
				t.Errorf("checkStatsHost() = %v, want %v", got, tt.want)
			}
		})
	}
}

// This test is only for a bad example as we test the working examples using testConfig()
func TestConfig_Validate(t *testing.T) {
	log = zap.NewExample(zap.AddCaller(), zap.WithCaller(true)).Named(provider)

	type fields struct {
		statsHost  *string
		statsPort  *string
		interval   *time.Duration
		pdnsHost   *string
		pdnsPort   *string
		pdnsAPIKey *string
		recursor   *bool
		StatsChan  chan Statistic
		Done       chan bool
		pdnsDone   chan bool
		statsDone  chan bool
	}
	tests := []struct {
		name     string
		fields   fields
		want     fields
		response bool
		wantErr  bool
	}{
		{
			name: "bad configuration",
			fields: fields{
				pdnsAPIKey: stringPtr(""),
			},
			want: fields{
				statsHost:  stringPtr("127.0.0.1"),
				statsPort:  stringPtr("8125"),
				interval:   timePtr(time.Duration(1) * time.Second),
				pdnsHost:   stringPtr("127.0.0.1"),
				pdnsPort:   stringPtr("8080"),
				pdnsAPIKey: stringPtr("x-api-key"),
				recursor:   boolPtr(true),
			},
			response: false,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := new(Config)
			if *tt.fields.pdnsAPIKey != "" {
				os.Setenv("PDNS_API_KEY", *tt.fields.pdnsAPIKey)
			}

			if got := c.Validate(); got != tt.response {
				t.Errorf("Config.Validate() = %v, want %v", got, tt.want)
			}
		})
	}
}
