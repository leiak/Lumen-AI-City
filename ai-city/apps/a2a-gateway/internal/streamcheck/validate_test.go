package streamcheck

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestValidator(t *testing.T) (*Validator, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return New(rdb, "http://localhost"), mr
}

// TestSubscribeBeatCollectsByNpcID is the smoke test from the plan: publish a
// single beat for npc_a after Subscribe confirms, then assert exactly one beat
// is returned for the matching npcID.
func TestSubscribeBeatCollectsByNpcID(t *testing.T) {
	v, mr := newTestValidator(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		// Give SubscribeBeat a moment to call sub.Receive() — without this the
		// Publish can race ahead of subscription confirmation and drop the msg.
		time.Sleep(100 * time.Millisecond)
		mr.Publish(ChannelSayStream, `{"type":"npc_say_stream","npc_id":"npc_a","sentence_idx":0,"text":"hi","emotion":"happy","ts_ms":1,"trace_id":"tr"}`)
	}()

	beats, err := v.SubscribeBeat(ctx, "npc_a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(beats) != 1 {
		t.Fatalf("expected 1 beat, got %d", len(beats))
	}
	if beats[0]["emotion"] != "happy" {
		t.Fatalf("expected emotion=happy, got %v", beats[0]["emotion"])
	}
}

// TestSubscribeBeatFiltersByNpcID ensures that beats for *other* NPCs are not
// returned even when present on the same channel — this is what lets the
// acceptance binary walk 5 NPCs without false positives.
func TestSubscribeBeatFiltersByNpcID(t *testing.T) {
	v, mr := newTestValidator(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		mr.Publish(ChannelSayStream, `{"type":"npc_say_stream","npc_id":"npc_other","sentence_idx":0,"text":"x","emotion":"neutral","ts_ms":1,"trace_id":"t1"}`)
		mr.Publish(ChannelSayStream, `{"type":"npc_say_stream","npc_id":"npc_target","sentence_idx":0,"text":"a","emotion":"neutral","ts_ms":2,"trace_id":"t2"}`)
		mr.Publish(ChannelSayStream, `{"type":"npc_say_stream","npc_id":"npc_target","sentence_idx":1,"text":"b","emotion":"happy","ts_ms":3,"trace_id":"t3"}`)
	}()

	beats, err := v.SubscribeBeat(ctx, "npc_target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(beats) != 2 {
		t.Fatalf("expected 2 beats for npc_target, got %d", len(beats))
	}
	if beats[0]["sentence_idx"].(float64) != 0 || beats[1]["sentence_idx"].(float64) != 1 {
		t.Fatalf("expected sentence_idx 0,1 order, got %v,%v", beats[0]["sentence_idx"], beats[1]["sentence_idx"])
	}
}

// TestSubscribeBeatSkipsMalformed makes sure a malformed payload on the
// channel does not crash the validator — defensive against stale messages.
func TestSubscribeBeatSkipsMalformed(t *testing.T) {
	v, mr := newTestValidator(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		mr.Publish(ChannelSayStream, `not-json`)
		mr.Publish(ChannelSayStream, `{"type":"npc_say_stream","npc_id":"npc_a","sentence_idx":0,"text":"ok","emotion":"neutral","ts_ms":1,"trace_id":"t"}`)
	}()

	beats, err := v.SubscribeBeat(ctx, "npc_a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(beats) != 1 {
		t.Fatalf("expected 1 beat (after skipping malformed), got %d", len(beats))
	}
}
