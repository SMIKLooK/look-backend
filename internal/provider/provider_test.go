package provider

import (
	"context"
	"strings"
	"testing"

	"look-backend/internal/domain"
)

type stubProvider struct {
	name   string
	models []string
}

func (s stubProvider) Name() string     { return s.name }
func (s stubProvider) Models() []string { return append([]string(nil), s.models...) }
func (s stubProvider) Complete(_ context.Context, _ domain.Request) (domain.Response, error) {
	return domain.Response{Provider: s.name}, nil
}

func (s stubProvider) Supports(model string) bool {
	for _, known := range s.models {
		if strings.EqualFold(known, model) {
			return true
		}
	}
	return false
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry(stubProvider{name: "один", models: []string{"m1"}})
	r.Register(stubProvider{name: "два", models: []string{"m2"}})

	infos := r.List()
	if len(infos) != 2 {
		t.Fatalf("ожидалось 2 провайдера, получено %d", len(infos))
	}

	r.Register(stubProvider{name: "два", models: []string{"m2-новая"}})
	infos = r.List()
	if len(infos) != 2 {
		t.Fatalf("после замены ожидалось 2 провайдера, получено %d", len(infos))
	}
	for _, info := range infos {
		if info.Name == "два" && len(info.Models) == 1 && info.Models[0] == "m2-новая" {
			return
		}
	}
	t.Error("замена провайдера не применилась")
}

func TestRegistry_Resolve_Priority(t *testing.T) {
	r := NewRegistry(
		stubProvider{name: "первый", models: []string{"общая"}},
		stubProvider{name: "второй", models: []string{"общая"}},
	)
	p, model, err := r.Resolve("Общая")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if p.Name() != "первый" {
		t.Errorf("ожидался первый зарегистрированный провайдер, получен %s", p.Name())
	}
	if model != "Общая" {
		t.Errorf("модель должна пройти насквозь без изменений, получено %q", model)
	}
}

func TestRegistry_Resolve_Alias(t *testing.T) {
	r := NewRegistry(stubProvider{name: "gemini", models: []string{"gemini-3.6-flash"}})
	r.SetAliases(map[string]string{"Gemeni": "gemini-3.6-flash"})

	p, model, err := r.Resolve("gEmEnI")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if p.Name() != "gemini" || model != "gemini-3.6-flash" {
		t.Errorf("ожидался gemini и каноническое имя, получены %s и %q", p.Name(), model)
	}
}

func TestRegistry_Resolve_WithoutAlias(t *testing.T) {
	r := NewRegistry(stubProvider{name: "gptunnel", models: []string{"gpt-4o"}})
	r.SetAliases(map[string]string{"чужой": "gpt-4o"})

	p, model, err := r.Resolve("GPT-4O")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if p.Name() != "gptunnel" || model != "GPT-4O" {
		t.Errorf("ожидался gptunnel и GPT-4O, получены %s и %q", p.Name(), model)
	}
}

func TestRegistry_Resolve_UnknownModel(t *testing.T) {
	r := NewRegistry(stubProvider{name: "gemini", models: []string{"gemini-3.6-flash"}})
	_, _, err := r.Resolve("gpt-4o")
	if err == nil {
		t.Fatal("ожидалась ошибка, получен успех")
	}
	if code := domain.ErrorCode(err); code != domain.CodeUnknownModel {
		t.Errorf("ожидался код %s, получен %s (%v)", domain.CodeUnknownModel, code, err)
	}
	if !strings.Contains(err.Error(), "gpt-4o") {
		t.Errorf("в ошибке должно быть имя запрошенной модели: %v", err)
	}
}

func TestRegistry_SetAliases_LowercaseAndCopy(t *testing.T) {
	r := NewRegistry()
	r.SetAliases(map[string]string{"ГЕМЕНИ": "gemini-3.6-flash"})

	aliases := r.Aliases()
	if aliases["гемени"] != "gemini-3.6-flash" {
		t.Fatalf("псевдоним должен храниться в нижнем регистре, получено %v", aliases)
	}

	aliases["вредный"] = "модель"
	if _, ok := r.Aliases()["вредный"]; ok {
		t.Error("Aliases вернул не копию: мутации видны в реестре")
	}
}

func TestRegistry_List_Sorted(t *testing.T) {
	r := NewRegistry(
		stubProvider{name: "zebra", models: []string{"m"}},
		stubProvider{name: "alpha", models: []string{"m"}},
	)
	infos := r.List()
	if len(infos) != 2 || infos[0].Name != "alpha" || infos[1].Name != "zebra" {
		t.Errorf("ожидался список, отсортированный по имени, получено %v", infos)
	}
}
