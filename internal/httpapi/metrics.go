package httpapi

import "sync/atomic"

type Metrics struct {
	requests atomic.Uint64
	failures atomic.Uint64
	inFlight atomic.Int64
}

func (m *Metrics) requestStarted() {
	m.requests.Add(1)
	m.inFlight.Add(1)
}

func (m *Metrics) requestFinished(failed bool) {
	m.inFlight.Add(-1)
	if failed {
		m.failures.Add(1)
	}
}
