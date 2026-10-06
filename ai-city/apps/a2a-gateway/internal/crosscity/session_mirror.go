// Package crosscity session_mirror.go —— A 城 a2a-gateway 持 sid 的镜像 store。
//
// 镜像 agent-os SessionStore 语义（A2）：60min TTL + 重连补帧 buffer + done flag。
// 不复用 Python 源：避免跨语言依赖；Go 重写一份保持 API 一致。
//
// 与 stage 2 差异：
//   - SID 由 B 城 agent-os 在 SayStreamForward init 帧生成，A 城原样接收并镜像
//   - 不依赖 Redis（纯进程内 map）；适合单实例 demo，多副本需换 Redis 或粘性路由
//   - 公开字段（如 SID / NPCID）方便 gRPC handler 直接序列化，无需再开 getter
package crosscity

import (
	"errors"
	"sync"
	"time"
)

// Beat 一帧节拍（cross-city stream 单元）；字段命名与 stage 2 JSON 一致。
type Beat struct {
	SentenceIdx uint32 `json:"sentence_idx"`
	Text        string `json:"text"`
	Emotion     string `json:"emotion"`
	TSMS        int64  `json:"ts_ms"`
}

// MirrorSession 跨城流 session（持有 sid 在 A 城 a2a-gateway 一侧）。
type MirrorSession struct {
	SID       string
	NPCID     string
	PlayerID  string
	CreatedAt time.Time
	Beats     []Beat
	Done      bool
	Complete  bool
	mu        sync.RWMutex
}

// ErrSessionNotFound session 不存在或 TTL 过期（对应 agent-os R015）。
var ErrSessionNotFound = errors.New("crosscity: session not found or expired")

// MirrorStore 跨城流 session 集合（A 城 a2a-gateway 持有）。
type MirrorStore struct {
	ttl      time.Duration
	mu       sync.Mutex
	sessions map[string]*MirrorSession
}

// NewMirrorStore 默认 60min TTL；传 0 也用 60min。
func NewMirrorStore(ttl time.Duration) *MirrorStore {
	if ttl == 0 {
		ttl = 60 * time.Minute
	}
	return &MirrorStore{
		ttl:      ttl,
		sessions: make(map[string]*MirrorSession),
	}
}

// Create 新建 session（先 evict 过期项）。返回会话指针，便于 gRPC handler
// 在创建后立即往 Beats 里追加 init prompt 这类初始化字段。
func (s *MirrorStore) Create(sid, npcID, playerID string) *MirrorSession {
	s.evictExpired()
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := &MirrorSession{
		SID:       sid,
		NPCID:     npcID,
		PlayerID:  playerID,
		CreatedAt: time.Now(),
		Beats:     make([]Beat, 0, 8),
	}
	s.sessions[sid] = sess
	return sess
}

// Append 写入一帧；session 不存在或 TTL 过期返 ErrSessionNotFound。
func (s *MirrorStore) Append(sid string, beat Beat) error {
	sess, err := s.get(sid)
	if err != nil {
		return err
	}
	sess.mu.Lock()
	sess.Beats = append(sess.Beats, beat)
	sess.mu.Unlock()
	return nil
}

// MarkDone 标记流结束；complete=false 表示异常结束（timeout / LLM 报错 / 断连）。
func (s *MirrorStore) MarkDone(sid string, complete bool) error {
	sess, err := s.get(sid)
	if err != nil {
		return err
	}
	sess.mu.Lock()
	sess.Done = true
	sess.Complete = complete
	sess.mu.Unlock()
	return nil
}

// Buffer 返回 sentence_idx >= fromIdx 的所有帧 + 当前 done 标志。
// session 不存在或过期返 ErrSessionNotFound。
// fromIdx > len(beats) 返空切片 + 当前 done 状态（不报错，
// 让客户端可以放心问“从最新帧开始拉”）。
func (s *MirrorStore) Buffer(sid string, fromIdx uint32) ([]Beat, bool, error) {
	sess, err := s.get(sid)
	if err != nil {
		return nil, false, err
	}
	sess.mu.RLock()
	defer sess.mu.RUnlock()
	if int(fromIdx) > len(sess.Beats) {
		return []Beat{}, sess.Done, nil
	}
	return sess.Beats[fromIdx:], sess.Done, nil
}

// IsComplete session 是否正常结束（非 timeout / error）。不存在或过期返 ErrSessionNotFound。
func (s *MirrorStore) IsComplete(sid string) (bool, error) {
	sess, err := s.get(sid)
	if err != nil {
		return false, err
	}
	sess.mu.RLock()
	defer sess.mu.RUnlock()
	return sess.Complete, nil
}

// Count 当前 session 数（metrics / test）。
func (s *MirrorStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// get 检查存在 + TTL，过期则删除并返 ErrSessionNotFound。
// 所有对外查询都走这里，保证“创建后 60min 内可见”语义和 stage 2 完全一致。
func (s *MirrorStore) get(sid string) (*MirrorSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sid]
	if !ok {
		return nil, ErrSessionNotFound
	}
	if time.Since(sess.CreatedAt) > s.ttl {
		delete(s.sessions, sid)
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// evictExpired 清扫过期项（Create 时调用，避免 map 无限增长）。
// 非定时任务，调用方主动触发；适合 demo 单实例流量模型。
func (s *MirrorStore) evictExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for sid, sess := range s.sessions {
		if now.Sub(sess.CreatedAt) > s.ttl {
			delete(s.sessions, sid)
		}
	}
}