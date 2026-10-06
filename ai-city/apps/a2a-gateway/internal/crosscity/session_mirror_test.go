// MirrorSessionStore 单元测试（B1-T02）—— 无外部依赖，纯 in-memory。
//
// 9 用例覆盖：
//   1. Create + Append + Buffer(0) → 全量帧
//   2. Buffer(fromIdx=2) → 截断
//   3. Buffer(unknown sid) → ErrSessionNotFound
//   4. Buffer(fromIdx 超界) → 空切片 + done 标志
//   5. MarkDone(complete=true) → done=true
//   6. IsComplete(complete=true) → true
//   7. IsComplete(complete=false) → false（异常结束）
//   8. TTL 过期 → ErrSessionNotFound
//   9. Count() → 反映 map 大小
package crosscity

import (
	"errors"
	"testing"
	"time"
)

func TestCreateAppendAndBuffer(t *testing.T) {
	store := NewMirrorStore(60 * time.Minute)
	store.Create("sess-1", "npc_b_wu", "player-1")
	for i := uint32(0); i < 3; i++ {
		err := store.Append("sess-1", Beat{SentenceIdx: i, Text: "hi", Emotion: "happy", TSMS: int64(i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	beats, done, err := store.Buffer("sess-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(beats) != 3 {
		t.Fatalf("got %d beats", len(beats))
	}
	if done {
		t.Fatal("done should be false")
	}
}

func TestBufferFromIdx(t *testing.T) {
	store := NewMirrorStore(60 * time.Minute)
	store.Create("sess-1", "npc", "p")
	for i := uint32(0); i < 5; i++ {
		if err := store.Append("sess-1", Beat{SentenceIdx: i, Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	beats, _, err := store.Buffer("sess-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(beats) != 3 {
		t.Fatalf("got %d beats from idx 2", len(beats))
	}
	if beats[0].SentenceIdx != 2 {
		t.Fatalf("first beat idx = %d, want 2", beats[0].SentenceIdx)
	}
}

func TestBufferUnknownSession(t *testing.T) {
	store := NewMirrorStore(60 * time.Minute)
	_, _, err := store.Buffer("missing", 0)
	if err != ErrSessionNotFound {
		t.Fatalf("got err=%v", err)
	}
}

func TestBufferOutOfRange(t *testing.T) {
	store := NewMirrorStore(60 * time.Minute)
	store.Create("sess-1", "npc", "p")
	if err := store.Append("sess-1", Beat{Text: "a"}); err != nil {
		t.Fatal(err)
	}
	beats, done, err := store.Buffer("sess-1", 99)
	if err != nil {
		t.Fatal(err)
	}
	if len(beats) != 0 {
		t.Fatalf("expected empty, got %d", len(beats))
	}
	if done {
		t.Fatal("not done yet")
	}
}

func TestMarkDone(t *testing.T) {
	store := NewMirrorStore(60 * time.Minute)
	store.Create("sess-1", "npc", "p")
	if err := store.MarkDone("sess-1", true); err != nil {
		t.Fatal(err)
	}
	_, done, err := store.Buffer("sess-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("done should be true after MarkDone")
	}
}

func TestIsComplete(t *testing.T) {
	store := NewMirrorStore(60 * time.Minute)
	store.Create("sess-1", "npc", "p")
	if err := store.MarkDone("sess-1", true); err != nil {
		t.Fatal(err)
	}
	complete, err := store.IsComplete("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if !complete {
		t.Fatal("complete should be true")
	}
}

func TestIsCompleteIncomplete(t *testing.T) {
	store := NewMirrorStore(60 * time.Minute)
	store.Create("sess-1", "npc", "p")
	if err := store.MarkDone("sess-1", false); err != nil {
		t.Fatal(err)
	}
	complete, err := store.IsComplete("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if complete {
		t.Fatal("complete should be false")
	}
}

func TestTTLExpiry(t *testing.T) {
	store := NewMirrorStore(50 * time.Millisecond)
	store.Create("sess-1", "npc", "p")
	time.Sleep(80 * time.Millisecond)
	_, _, err := store.Buffer("sess-1", 0)
	if err != ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestCount(t *testing.T) {
	store := NewMirrorStore(60 * time.Minute)
	if store.Count() != 0 {
		t.Fatal("empty should be 0")
	}
	store.Create("a", "n", "p")
	store.Create("b", "n", "p")
	if store.Count() != 2 {
		t.Fatal("expected 2")
	}
}

func TestGetReturnsSession(t *testing.T) {
	store := NewMirrorStore(time.Minute)
	store.Create("s1", "npc_b_wu", "p1")
	sess, err := store.Get("s1")
	if err != nil {
		t.Fatal(err)
	}
	if sess.NPCID != "npc_b_wu" {
		t.Fatalf("npc_id=%s", sess.NPCID)
	}
	if sess.PlayerID != "p1" {
		t.Fatalf("player_id=%s", sess.PlayerID)
	}
}

func TestGetMissingReturnsErr(t *testing.T) {
	store := NewMirrorStore(time.Minute)
	_, err := store.Get("missing")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestGetExpiredReturnsErr(t *testing.T) {
	store := NewMirrorStore(30 * time.Millisecond)
	store.Create("s1", "npc", "p")
	time.Sleep(60 * time.Millisecond)
	_, err := store.Get("s1")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("got %v", err)
	}
}