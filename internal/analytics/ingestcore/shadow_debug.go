package ingestcore

import (
	"sync/atomic"
)

// ShadowDebugCounters tracks optional ingest shadow diagnostics (INGEST_SHADOW_DEBUG=1).
type ShadowDebugCounters struct {
	RawLines          atomic.Uint64
	ParsedPrivmsg     atomic.Uint64
	CountedMessages   atomic.Uint64
	ParseFailures     atomic.Uint64
	MissingStreamBind atomic.Uint64
	NonPrivmsgIgnored atomic.Uint64
	AllowlistRejected atomic.Uint64
	EnqueueDropped    atomic.Uint64
}

var shadowDebug ShadowDebugCounters

// ShadowDebugEnabled reports whether shadow debug counters are active.
func ShadowDebugEnabled() bool {
	return shadowDebugActive
}

var shadowDebugActive bool

func setShadowDebugActive(on bool) {
	shadowDebugActive = on
}

// ShadowDebugSnapshot returns a copy of current debug counter values.
func ShadowDebugSnapshot() map[string]uint64 {
	return map[string]uint64{
		"raw_lines":           shadowDebug.RawLines.Load(),
		"parsed_privmsg":      shadowDebug.ParsedPrivmsg.Load(),
		"counted_messages":    shadowDebug.CountedMessages.Load(),
		"parse_failures":      shadowDebug.ParseFailures.Load(),
		"missing_stream_bind": shadowDebug.MissingStreamBind.Load(),
		"non_privmsg_ignored": shadowDebug.NonPrivmsgIgnored.Load(),
		"allowlist_rejected":  shadowDebug.AllowlistRejected.Load(),
		"enqueue_dropped":     shadowDebug.EnqueueDropped.Load(),
	}
}
