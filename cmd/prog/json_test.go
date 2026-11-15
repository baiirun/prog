package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTimestamp_MarshalsAsUTCWholeSeconds(t *testing.T) {
	// A time stored with a local offset and sub-second precision must marshal
	// in the one form jq's fromdateiso8601 accepts.
	cst := time.FixedZone("CST", -6*60*60)
	in := time.Date(2026, 1, 9, 15, 12, 45, 123456789, cst)

	b, err := json.Marshal(Timestamp(in))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := string(b), `"2026-01-09T21:12:45Z"`; got != want {
		t.Errorf("marshal = %s, want %s", got, want)
	}

	var back Timestamp
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !time.Time(back).Equal(in.Truncate(time.Second)) {
		t.Errorf("round-trip changed the instant: got %v, want %v", time.Time(back), in)
	}
}
