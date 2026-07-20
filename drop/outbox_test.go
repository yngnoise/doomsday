package drop

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOutboxBackoff(t *testing.T) {
	initial := time.Second
	maximum := 5 * time.Second
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: time.Second},
		{attempt: 2, want: 2 * time.Second},
		{attempt: 3, want: 4 * time.Second},
		{attempt: 4, want: 5 * time.Second},
		{attempt: 20, want: 5 * time.Second},
	}
	for _, test := range cases {
		if got := outboxBackoff(test.attempt, initial, maximum); got != test.want {
			t.Errorf("attempt %d: got %s, want %s", test.attempt, got, test.want)
		}
	}
}

func TestTruncateOutboxError(t *testing.T) {
	got := truncateOutboxError(errors.New(strings.Repeat("x", 2500)))
	if len(got) != 2000 {
		t.Fatalf("error length = %d, want 2000", len(got))
	}
}
