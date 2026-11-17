package modelcap

import "testing"

func TestManualCompactionThreshold(t *testing.T) {
	for _, test := range []struct {
		used, window int64
		want         bool
	}{{0, 100000, false}, {8191, 32768, false}, {8192, 32768, true}, {9999, 100000, false}, {10000, 100000, true}, {17040, 1000000, false}, {100000, 1000000, true}, {10000, 100001, false}, {10001, 100001, true}} {
		u := ContextUsage{UsedTokens: &test.used, ContextWindow: test.window}
		if got := ManualCompactionEligible(&u); got != test.want {
			t.Fatalf("used=%d window=%d eligible=%v", test.used, test.window, got)
		}
	}
	if ManualCompactionEligible(nil) || ManualCompactionEligible(&ContextUsage{ContextWindow: 100000}) {
		t.Fatal("unknown usage is not zero")
	}
}
