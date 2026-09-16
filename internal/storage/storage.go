// Package storage — хранилища истории диалогов.
//
// Store — контракт между сервисом и местом хранения: память, SQLite,
// PostgreSQL. Сервис не знает деталей; чтобы включить персистентность,
// достаточно реализовать Store поверх выбранной БД и передать реализацию
// в service.New (см. internal/storage/memory.go как образец).
package storage

import "look-backend/internal/domain"

// Store — хранилище истории диалогов по сессиям.
type Store interface {
	// Append добавляет сообщение в конец истории сессии.
	Append(sessionID string, msg domain.Message) error
	// History возвращает копию истории сессии; для неизвестной сессии —
	// пустой список и nil.
	History(sessionID string) ([]domain.Message, error)
	// Reset полностью очищает историю сессии.
	Reset(sessionID string) error
}
