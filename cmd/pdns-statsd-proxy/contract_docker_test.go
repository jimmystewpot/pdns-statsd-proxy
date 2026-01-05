//go:build contract

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	contractAPIKey = "changeme"
)

type dockerContainer struct {
	id string
}

func (c *dockerContainer) Stop(ctx context.Context, t *testing.T) {
	t.Helper()
	if c == nil || c.id == "" {
		return
	}
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", c.id).Run()
}

func requireContractEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("PDNS_CONTRACT") != "1" {
		t.Skip("PDNS_CONTRACT is not set to 1")
	}
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skipf("docker is not available: %v", err)
	}
}

func dockerRun(ctx context.Context, t *testing.T, image string, args ...string) (*dockerContainer, error) {
	t.Helper()

	cmdArgs := []string{"run", "-d", "--rm"}
	cmdArgs = append(cmdArgs, args...)
	cmdArgs = append(cmdArgs, image)

	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker run failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	id := strings.TrimSpace(string(out))
	return &dockerContainer{id: id}, nil
}

func dockerPort(ctx context.Context, t *testing.T, id string, containerPort string) (string, error) {
	t.Helper()
	out, err := exec.CommandContext(ctx, "docker", "port", id, containerPort).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker port failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	// output like: 0.0.0.0:49153 or 127.0.0.1:49153
	return strings.TrimSpace(string(out)), nil
}

func waitForHTTP(ctx context.Context, t *testing.T, url string, headers map[string]string) error {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if err != nil {
			return err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s: %w", url, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func newContractConfig(host string, port string) *Config {
	interval := 1 * time.Second
	recursor := true
	return &Config{
		statsHost:               stringPtr("127.0.0.1"),
		statsPort:               stringPtr("8125"),
		interval:                &interval,
		pdnsHost:                &host,
		pdnsPort:                &port,
		pdnsAPIKey:              stringPtr(contractAPIKey),
		recursor:                &recursor,
		counterCumulativeValues: make(map[string]int64),
		StatsChan:               make(chan Statistic, 1000),
		stop:                    make(chan struct{}),
		pdnsExited:              make(chan struct{}),
		statsExited:             make(chan struct{}),
	}
}

func TestContract_Recursor_Pre43_LegacyStatistics(t *testing.T) {
	requireContractEnabled(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	image := os.Getenv("PDNS_RECURSOR_PRE43_IMAGE")
	if image == "" {
		image = "powerdns/pdns-recursor-42:4.2.0"
	}

	c, err := dockerRun(ctx, t, image,
		"-p", "127.0.0.1::8082",
		"--name", fmt.Sprintf("pdns-recursor-pre43-%d", time.Now().UnixNano()),
		"--",
		"pdns_recursor",
		"--local-address=0.0.0.0",
		"--webserver=yes",
		"--webserver-address=0.0.0.0",
		"--webserver-port=8082",
		"--api-key="+contractAPIKey,
	)
	if err != nil {
		t.Skipf("unable to start recursor pre-4.3 container (%s): %v", image, err)
	}
	defer c.Stop(ctx, t)

	mapped, err := dockerPort(ctx, t, c.id, "8082/tcp")
	if err != nil {
		t.Skipf("unable to discover mapped port: %v", err)
	}
	_, port, err := net.SplitHostPort(mapped)
	if err != nil {
		t.Fatalf("unexpected docker port output %q: %v", mapped, err)
	}

	apiURL := fmt.Sprintf("http://127.0.0.1:%s/api/v1/servers/localhost/statistics", port)
	if err := waitForHTTP(ctx, t, apiURL, map[string]string{"X-API-Key": contractAPIKey}); err != nil {
		t.Fatalf("recursor API never became ready: %v", err)
	}

	config := newContractConfig("127.0.0.1", port)
	pdns := new(pdnsClient)
	pdns.Initialise(config)

	resp, err := pdns.Poll()
	if err != nil {
		t.Fatalf("pdns.Poll() error = %v", err)
	}
	defer resp.Body.Close()

	if pdns.usePrometheus {
		t.Fatalf("expected pre-4.3 to use legacy JSON, but usePrometheus=true")
	}
	if resp.Header.Get("Content-Type") == "text/plain; version=0.0.4" {
		t.Fatalf("unexpected prometheus content-type for pre-4.3")
	}

	// Decode should succeed for legacy JSON.
	resp.Body.Close()
	resp, err = pdns.doRequest(pdns.legacyPath)
	if err != nil {
		t.Fatalf("pdns legacy doRequest error = %v", err)
	}
	if err := decodeStats(resp, config); err != nil {
		t.Fatalf("decodeStats() error = %v", err)
	}
	if len(config.StatsChan) == 0 {
		t.Fatalf("expected some statistics to be decoded")
	}
}

func TestContract_Recursor_43Plus_PrometheusMetrics(t *testing.T) {
	requireContractEnabled(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	image := os.Getenv("PDNS_RECURSOR_43PLUS_IMAGE")
	if image == "" {
		image = "powerdns/pdns-recursor-49:4.9.0"
	}

	c, err := dockerRun(ctx, t, image,
		"-p", "127.0.0.1::8082",
		"--name", fmt.Sprintf("pdns-recursor-43plus-%d", time.Now().UnixNano()),
		"--",
		"pdns_recursor",
		"--local-address=0.0.0.0",
		"--webserver=yes",
		"--webserver-address=0.0.0.0",
		"--webserver-port=8082",
		"--api-key="+contractAPIKey,
	)
	if err != nil {
		t.Skipf("unable to start recursor 4.3+ container (%s): %v", image, err)
	}
	defer c.Stop(ctx, t)

	mapped, err := dockerPort(ctx, t, c.id, "8082/tcp")
	if err != nil {
		t.Skipf("unable to discover mapped port: %v", err)
	}
	_, port, err := net.SplitHostPort(mapped)
	if err != nil {
		t.Fatalf("unexpected docker port output %q: %v", mapped, err)
	}

	apiURL := fmt.Sprintf("http://127.0.0.1:%s/api/v1/servers/localhost", port)
	if err := waitForHTTP(ctx, t, apiURL, map[string]string{"X-API-Key": contractAPIKey}); err != nil {
		t.Fatalf("recursor API never became ready: %v", err)
	}

	config := newContractConfig("127.0.0.1", port)
	pdns := new(pdnsClient)
	pdns.Initialise(config)

	resp, err := pdns.Poll()
	if err != nil {
		t.Fatalf("pdns.Poll() error = %v", err)
	}

	if !pdns.usePrometheus {
		t.Fatalf("expected 4.3+ to use /metrics, but usePrometheus=false")
	}

	buf := new(bytes.Buffer)
	io.CopyN(buf, resp.Body, 256)
	resp.Body.Close()
	if !strings.Contains(buf.String(), "#") {
		t.Fatalf("expected prometheus text output to contain comments, got: %q", buf.String())
	}

	resp, err = pdns.doRequest(pdns.prometheusPath)
	if err != nil {
		t.Fatalf("pdns prometheus doRequest error = %v", err)
	}
	if err := decodePrometheusStats(resp, config); err != nil {
		t.Fatalf("decodePrometheusStats() error = %v", err)
	}
	if len(config.StatsChan) == 0 {
		t.Fatalf("expected some prometheus statistics to be decoded")
	}
}

// Note: Authoritative contract tests are intentionally omitted here because the
// official container requires backend configuration that is not yet standardised
// in this repository. Once we decide on a supported auth backend (e.g. gsqlite3
// with schema init), we can add analogous tests for pdns_auth.
