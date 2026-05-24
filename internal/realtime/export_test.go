package realtime

// PublishForTest exposes the unexported publish path so external
// _test packages can drive the hub deterministically without a
// real Postgres NOTIFY round-trip. Only compiled into test binaries.
func PublishForTest(h *Hub, ev Event) { h.publish(ev) }
