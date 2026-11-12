// Package metrics is a small, dependency-free metrics registry. It is
// deliberately not the official prometheus/client_golang library: Cinc
// Collector only needs a handful of counters and gauges, and emitting
// Prometheus text exposition format directly keeps the binary free of a
// heavyweight client library, mirroring cinc-server-ng's own approach.
package metrics

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
)

// Registry holds a set of named counters and gauges and renders them as
// Prometheus text exposition format.
type Registry struct {
	mu       sync.Mutex
	counters map[string]*Counter
	gauges   map[string]*GaugeFunc
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		counters: make(map[string]*Counter),
		gauges:   make(map[string]*GaugeFunc),
	}
}

// Counter is a monotonically increasing value.
type Counter struct {
	v    int64
	help string
}

// Inc increments the counter by 1.
func (c *Counter) Inc() { atomic.AddInt64(&c.v, 1) }

// Add increments the counter by delta.
func (c *Counter) Add(delta int64) { atomic.AddInt64(&c.v, delta) }

// Value returns the current counter value.
func (c *Counter) Value() int64 { return atomic.LoadInt64(&c.v) }

// GaugeFunc is a gauge whose value is computed on read.
type GaugeFunc struct {
	fn   func() float64
	help string
}

// Counter returns the named counter, creating it if necessary.
func (r *Registry) Counter(name, help string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[name]; ok {
		return c
	}
	c := &Counter{help: help}
	r.counters[name] = c
	return c
}

// GaugeFunc registers a gauge whose value is computed by fn each time
// metrics are rendered.
func (r *Registry) GaugeFunc(name, help string, fn func() float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gauges[name] = &GaugeFunc{fn: fn, help: help}
}

// WriteText renders the registry in Prometheus text exposition format
// (version 0.0.4).
func (r *Registry) WriteText(w io.Writer) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	names := make([]string, 0, len(r.counters))
	for name := range r.counters {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c := r.counters[name]
		if c.help != "" {
			if _, err := fmt.Fprintf(w, "# HELP %s %s\n", name, c.help); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "# TYPE %s counter\n%s %s\n", name, name, strconv.FormatInt(c.Value(), 10)); err != nil {
			return err
		}
	}

	gaugeNames := make([]string, 0, len(r.gauges))
	for name := range r.gauges {
		gaugeNames = append(gaugeNames, name)
	}
	sort.Strings(gaugeNames)
	for _, name := range gaugeNames {
		g := r.gauges[name]
		if g.help != "" {
			if _, err := fmt.Fprintf(w, "# HELP %s %s\n", name, g.help); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "# TYPE %s gauge\n%s %s\n", name, name, strconv.FormatFloat(g.fn(), 'g', -1, 64)); err != nil {
			return err
		}
	}
	return nil
}
