package triggers

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

func TestStartCron_FiresDueSchedules(t *testing.T) {
	var mu sync.Mutex
	var fired []Run
	tr := New(lineage.NewMinter(), func(r Run) {
		mu.Lock()
		fired = append(fired, r)
		mu.Unlock()
	})
	tr.RegisterCron("mtg", "@every 1s")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n, err := tr.StartCron(ctx)
	if err != nil {
		t.Fatalf("StartCron: %v", err)
	}
	if n != 1 {
		t.Fatalf("watched schedules: got %d want 1", n)
	}

	// Two fire windows plus slack.
	time.Sleep(2500 * time.Millisecond)
	cancel()

	mu.Lock()
	defer mu.Unlock()
	if len(fired) < 2 {
		t.Fatalf("expected >=2 cron fires in 2.5s at @every 1s, got %d", len(fired))
	}
	for _, r := range fired {
		if r.Service != "mtg" || r.Kind != KindCron {
			t.Errorf("fired run: %+v", r)
		}
		if r.Stamp.Sequence == "" || r.Stamp.SequenceOrder != 0 {
			t.Errorf("cron fire must open a new sequence at order 0: %+v", r.Stamp)
		}
	}
	// Each fire is a NEW sequence (external trigger = chain head).
	if fired[0].Stamp.Sequence == fired[1].Stamp.Sequence {
		t.Error("consecutive cron fires must not share a sequence")
	}
}

func TestStartCron_BadSpecFailsStart(t *testing.T) {
	tr := New(lineage.NewMinter(), nil)
	tr.RegisterCron("mtg", "not a cron")
	if _, err := tr.StartCron(context.Background()); err == nil {
		t.Fatal("a bad cron spec must fail the start")
	}
}

func TestStartCron_NoSchedules(t *testing.T) {
	tr := New(lineage.NewMinter(), nil)
	n, err := tr.StartCron(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("empty start: n=%d err=%v", n, err)
	}
}

func TestParseCron_Timezone(t *testing.T) {
	if _, err := ParseCron("CRON_TZ=Asia/Tokyo 0 9 * * *"); err != nil {
		t.Errorf("CRON_TZ prefix should parse: %v", err)
	}
	if _, err := ParseCron("0 9 * * *"); err != nil {
		t.Errorf("plain 5-field should parse: %v", err)
	}
	if _, err := ParseCron("@hourly"); err != nil {
		t.Errorf("descriptor should parse: %v", err)
	}
}
