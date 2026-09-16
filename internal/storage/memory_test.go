package storage

import (
	"testing"
	"time"

	"look-backend/internal/domain"
)

func TestMemoryStore_AppendHistoryReset(t *testing.T) {
	store := NewMemoryStore(time.Hour, time.Minute)

	if msgs, err := store.History("missing"); err != nil || len(msgs) != 0 {
		t.Fatalf("неизвестная сессия должна возвращать пустую историю: %v %v", msgs, err)
	}

	if err := store.Append("s1", domain.Message{Role: "user", Content: "раз"}); err != nil {
		t.Fatalf("неожиданная ошибка Append: %v", err)
	}
	if err := store.Append("s1", domain.Message{Role: "assistant", Content: "два"}); err != nil {
		t.Fatalf("неожиданная ошибка Append: %v", err)
	}

	msgs, err := store.History("s1")
	if err != nil {
		t.Fatalf("неожиданная ошибка History: %v", err)
	}
	if len(msgs) != 2 || msgs[0].Content != "раз" || msgs[1].Content != "два" {
		t.Fatalf("неожиданная история: %+v", msgs)
	}

	// History возвращает копию: изменение слайса не должно влиять на хранилище
	msgs[0].Content = "hack"
	msgs, _ = store.History("s1")
	if msgs[0].Content != "раз" {
		t.Fatal("History вернул ссылку на внутренние данные вместо копии")
	}

	if err := store.Reset("s1"); err != nil {
		t.Fatalf("неожиданная ошибка Reset: %v", err)
	}
	if msgs, _ = store.History("s1"); len(msgs) != 0 {
		t.Fatalf("после Reset история должна быть пуста: %+v", msgs)
	}
}

func TestMemoryStore_PurgeExpiredSessions(t *testing.T) {
	store := NewMemoryStore(time.Millisecond, time.Millisecond)

	if err := store.Append("old", domain.Message{Role: "user", Content: "старое"}); err != nil {
		t.Fatalf("неожиданная ошибка Append: %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	// новая запись запускает ленивую очистку просроченных сессий
	if err := store.Append("fresh", domain.Message{Role: "user", Content: "свежее"}); err != nil {
		t.Fatalf("неожиданная ошибка Append: %v", err)
	}

	if msgs, _ := store.History("old"); len(msgs) != 0 {
		t.Fatalf("просроченная сессия должна была удалиться: %+v", msgs)
	}
	if msgs, _ := store.History("fresh"); len(msgs) != 1 {
		t.Fatalf("свежая сессия не должна была удалиться: %+v", msgs)
	}
}
