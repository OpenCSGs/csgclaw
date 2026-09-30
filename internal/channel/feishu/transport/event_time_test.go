package transport

import (
	"strconv"
	"testing"
	"time"
)

func TestEventTimestampUnits(t *testing.T) {
	expected := time.Date(2026, 9, 25, 7, 24, 47, 0, time.UTC)
	for _, value := range []int64{expected.Unix(), expected.UnixMilli(), expected.UnixMicro(), expected.UnixNano()} {
		if got := parseMillis(strconv.FormatInt(value, 10)); !got.Equal(expected) {
			t.Fatalf("timestamp %d parsed as %s, want %s", value, got, expected)
		}
	}
}
