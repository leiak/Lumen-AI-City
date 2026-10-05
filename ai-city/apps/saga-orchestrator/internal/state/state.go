// ai-city/apps/saga-orchestrator/internal/state/state.go
package state

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type SagaStatus string

const (
	StatusPending     SagaStatus = "PENDING"
	StatusRunning     SagaStatus = "RUNNING"
	StatusSuccess     SagaStatus = "SUCCESS"
	StatusCompensated SagaStatus = "COMPENSATED"
	StatusFailed      SagaStatus = "FAILED"
)

type SagaStep struct {
	WorkerID  string         `json:"worker_id"`
	Action    string         `json:"action"`
	Payload   map[string]any `json:"payload"`
	Status    SagaStatus     `json:"status"`
	StartedAt int64          `json:"started_at_ms"`
	EndedAt   int64          `json:"ended_at_ms"`
	Error     string         `json:"error,omitempty"`
}

type Saga struct {
	ID        string     `json:"saga_id"`
	PlayerID  string     `json:"player_id"`
	Steps     []SagaStep `json:"steps"`
	Status    SagaStatus `json:"status"`
	CreatedAt int64      `json:"created_at_ms"`
	UpdatedAt int64      `json:"updated_at_ms"`
	Result    string     `json:"result,omitempty"`
}

type Store struct {
	mu    sync.RWMutex
	sagas map[string]*Saga
}

func NewStore() *Store {
	return &Store{sagas: make(map[string]*Saga)}
}

func (s *Store) Create(id, playerID string, steps []SagaStep) *Saga {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	saga := &Saga{
		ID:        id,
		PlayerID:  playerID,
		Steps:     steps,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.sagas[id] = saga
	return saga
}

func (s *Store) Get(id string) (*Saga, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	saga, ok := s.sagas[id]
	if !ok {
		return nil, fmt.Errorf("saga %s not found", id)
	}
	return saga, nil
}

func (s *Store) UpdateStatus(id string, status SagaStatus, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	saga, ok := s.sagas[id]
	if !ok {
		return fmt.Errorf("saga %s not found", id)
	}
	saga.Status = status
	saga.UpdatedAt = time.Now().UnixMilli()
	saga.Result = result
	return nil
}

func (s *Store) UpdateStep(id string, idx int, step SagaStep) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	saga, ok := s.sagas[id]
	if !ok {
		return fmt.Errorf("saga %s not found", id)
	}
	if idx < 0 || idx >= len(saga.Steps) {
		return fmt.Errorf("step %d out of range", idx)
	}
	saga.Steps[idx] = step
	saga.UpdatedAt = time.Now().UnixMilli()
	return nil
}

// MarshalSaga 序列化为 JSON
func MarshalSaga(saga *Saga) ([]byte, error) {
	return json.Marshal(saga)
}

// CanTransition 状态机转换检查（用于补偿）
func CanTransition(from, to SagaStatus) bool {
	switch from {
	case StatusPending:
		return to == StatusRunning || to == StatusFailed
	case StatusRunning:
		return to == StatusSuccess || to == StatusCompensated || to == StatusFailed
	default:
		return false
	}
}