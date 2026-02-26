// Package webhook delivers analysis results to configured HTTP endpoints.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/bullarcdev/bullarc"
)

// Dispatcher sends AnalysisResult payloads to a list of webhook URLs via HTTP POST.
type Dispatcher struct {
	urls   []string
	client *http.Client
}

// event is the JSON payload sent to each webhook target.
type event struct {
	Symbol    string           `json:"symbol"`
	Timestamp time.Time        `json:"timestamp"`
	Signals   []bullarc.Signal `json:"signals"`
}

// New creates a Dispatcher that will POST to each of the given URLs.
// timeout controls how long to wait for each individual POST response.
// If timeout is zero, a 10-second default is used.
func New(urls []string, timeout time.Duration) *Dispatcher {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Dispatcher{
		urls:   urls,
		client: &http.Client{Timeout: timeout},
	}
}

// Dispatch serialises result and POSTs it to every configured URL.
// Failures are logged; if any target fails, a combined error is returned.
func (d *Dispatcher) Dispatch(ctx context.Context, result bullarc.AnalysisResult) error {
	if len(d.urls) == 0 {
		return nil
	}
	payload := event{
		Symbol:    result.Symbol,
		Timestamp: result.Timestamp,
		Signals:   result.Signals,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: marshal: %w", err)
	}

	var failed int
	for _, url := range d.urls {
		if err := d.post(ctx, url, data); err != nil {
			slog.Warn("webhook: dispatch failed", "url", url, "err", err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("webhook: %d/%d targets failed", failed, len(d.urls))
	}
	return nil
}

func (d *Dispatcher) post(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
