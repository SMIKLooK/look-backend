package provider

import (
	"context"
	"sort"
	"strings"
	"sync"

	"look-backend/internal/domain"
)

type Provider interface {
	Name() string
	// Models — модели, которые провайдер обслуживает по умолчанию.
	Models() []string
	// Supports — обслуживает ли провайдер указанную модель.
	Supports(model string) bool
	// Complete отправляет запрос и возвращает ответ модели.
	Complete(ctx context.Context, req domain.Request) (domain.Response, error)
}

// Info — описание провайдера для служебных эндпоинтов.
type Info struct {
	Name   string   `json:"name"`
	Models []string `json:"models"`
}

// Registry хранит провайдеров, псевдонимы моделей и подбирает провайдера.
type Registry struct {
	mu        sync.RWMutex
	providers []Provider
	aliases   map[string]string // псевдоним (в нижнем регистре) → каноническая модель
}

// NewRegistry создаёт реестр и регистрирует переданных провайдеров.
func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{}
	for _, p := range providers {
		r.Register(p)
	}
	return r
}

// Register добавляет провайдера; повторная регистрация с тем же именем
// заменяет прежнюю реализацию.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, existing := range r.providers {
		if existing.Name() == p.Name() {
			r.providers[i] = p
			return
		}
	}
	r.providers = append(r.providers, p)
}

// SetAliases задаёт псевдонимы моделей: короткое имя из текста
// (например "gemeni") → каноническое имя модели, которое уйдёт провайдеру.
func (r *Registry) SetAliases(aliases map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aliases = make(map[string]string, len(aliases))
	for alias, model := range aliases {
		r.aliases[strings.ToLower(alias)] = model
	}
}

// Aliases возвращает копию карты псевдонимов.
func (r *Registry) Aliases() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.aliases))
	for alias, model := range r.aliases {
		out[alias] = model
	}
	return out
}

// Resolve возвращает провайдера, обслуживающего модель, и каноническое
// имя модели — результат применения псевдонимов.
func (r *Registry) Resolve(model string) (Provider, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	target := model
	if canonical, ok := r.aliases[strings.ToLower(model)]; ok {
		target = canonical
	}
	for _, p := range r.providers {
		if p.Supports(target) {
			return p, target, nil
		}
	}
	return nil, "", domain.NewError(domain.CodeUnknownModel, "неизвестная модель: "+model)
}

// List возвращает описание всех зарегистрированных провайдеров.
func (r *Registry) List() []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]Info, 0, len(r.providers))
	for _, p := range r.providers {
		infos = append(infos, Info{Name: p.Name(), Models: p.Models()})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos
}
