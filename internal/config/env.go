package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Файл собран из читателей переменных окружения: каждый возвращает
// значение по умолчанию для пустой или отсутствующей переменной
// и ошибку с именем переменной, если значение не разбирается.

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envList читает список значений через запятую; пустая или отсутствующая
// переменная даёт значение по умолчанию.
func envList(key string, def []string) ([]string, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def, nil
	}
	return out, nil
}

func envDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

func envInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

// envBool читает булеву переменную (1, t, true, yes, y, on); пустая или
// отсутствующая переменная даёт значение по умолчанию.
func envBool(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf(`%s: ожидается true или false (сейчас %q)`, key, v)
	}
	return b, nil
}

func envInt64(key string, def int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

func envAliases(key string, def map[string]string) (map[string]string, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	out := make(map[string]string)
	for _, pair := range strings.Split(v, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		alias, model, found := strings.Cut(pair, "=")
		alias, model = strings.TrimSpace(alias), strings.TrimSpace(model)
		if !found || alias == "" || model == "" {
			return nil, fmt.Errorf(`%s: ожидается формат "псевдоним=модель" (сейчас %q)`, key, pair)
		}
		out[alias] = model
	}
	if len(out) == 0 {
		return def, nil
	}
	return out, nil
}
