package storage

import (
	"sync"
	"time"

	"look-backend/internal/domain"
)

// MemoryStore — хранилище истории в памяти процесса: быстро и без
// зависимостей, но история пропадает при перезапуске сервера и не делится
// между несколькими инстансами. Неактивные сессии удаляются ленивой
// очисткой (раз в purgeEvery, если сессия молчит дольше ttl).
type MemoryStore struct {
	mu         sync.RWMutex
	sessions   map[string]*memorySession
	ttl        time.Duration
	purgeEvery time.Duration
	lastPurge  time.Time
}

type memorySession struct {
	messages []domain.Message
	lastSeen time.Time
}

// NewMemoryStore создаёт хранилище; ttl и purgeEvery <= 0 заменяются
// на значения по умолчанию (24 часа / 10 минут).
func NewMemoryStore(sessionTTL, purgeEvery time.Duration) *MemoryStore {
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}
	if purgeEvery <= 0 {
		purgeEvery = 10 * time.Minute
	}
	return &MemoryStore{
		sessions:   make(map[string]*memorySession),
		ttl:        sessionTTL,
		purgeEvery: purgeEvery,
	}
}

func (s *MemoryStore) Append(sessionID string, msg domain.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[sessionID]
	if sess == nil {
		sess = &memorySession{}
		s.sessions[sessionID] = sess
	}
	sess.messages = append(sess.messages, msg)
	sess.lastSeen = time.Now()
	s.purgeLocked(time.Now())
	return nil
}

func (s *MemoryStore) History(sessionID string) ([]domain.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess := s.sessions[sessionID]
	if sess == nil {
		return nil, nil
	}
	out := make([]domain.Message, len(sess.messages))
	copy(out, sess.messages)
	return out, nil
}

func (s *MemoryStore) Reset(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}

func (s *MemoryStore) purgeLocked(now time.Time) {
	if now.Sub(s.lastPurge) < s.purgeEvery {
		return
	}
	s.lastPurge = now
	for id, sess := range s.sessions {
		if now.Sub(sess.lastSeen) > s.ttl {
			delete(s.sessions, id)
		}
	}
}
