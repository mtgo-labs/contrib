package netpoll

import (
	"testing"
	"time"
)

func TestNewDialerNormalizesReadSpin(t *testing.T) {
	tests := []struct {
		name string
		spin time.Duration
		want time.Duration
	}{
		{name: "disabled", spin: 0, want: 0},
		{name: "enabled", spin: 50 * time.Microsecond, want: 50 * time.Microsecond},
		{name: "negative_disabled", spin: -time.Microsecond, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dialer := NewDialer(Config{ReadSpin: test.spin})
			if dialer.cfg.ReadSpin != test.want {
				t.Fatalf("read spin = %v, want %v", dialer.cfg.ReadSpin, test.want)
			}
		})
	}
}
