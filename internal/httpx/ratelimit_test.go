package httpx

import (
	"testing"
	"time"
)

func TestRateLimiterBurstAndRefill(t *testing.T) {
	rl := NewRateLimiter(100, 3)
	for i := 0; i < 3; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("request %d within burst should be allowed", i)
		}
	}
	if rl.allow("1.2.3.4") {
		t.Fatal("4th request should be denied")
	}
	if !rl.allow("5.6.7.8") {
		t.Fatal("different ip should have its own bucket")
	}
	time.Sleep(20 * time.Millisecond)
	if !rl.allow("1.2.3.4") {
		t.Fatal("bucket should refill after wait")
	}
}
