package main

import (
	"fmt"
	"net/http"
	"time"
)

// probeStatus is used as the Docker HEALTHCHECK command and can double as a
// Kubernetes exec probe: it self-probes GET /_status and exits non-zero on
// anything but 200, since distroless images have no shell or curl to do
// this with an ordinary command.
func probeStatus(addr string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(addr)
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: unhealthy status %d", resp.StatusCode)
	}
	return nil
}
