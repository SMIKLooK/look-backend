package service

import (
	"context"
	"testing"
	"time"

	"look-backend/internal/domain"
	"look-backend/internal/parser"
	"look-backend/internal/provider"
)

type fakeProvider struct {
	name   string
	models []string
	sleep  time.Duration
	resp   domain.Response
	err    error

	gotReq domain.Request
}

func (f *fakeProvider) Name() string     { return f.name }
func (f *fakeProvider) Models() []string { return append([]string(nil), f.models...) }

func (f *fakeProvider) Supports(model string) bool {
	for _, known := range f.models {
		if known == model {
			return true
		}
	}
	return false
}

func (f *fakeProvider) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	f.gotReq = req
	if f.sleep > 0 {
		select {
		case <-time.After(f.sleep):
		case <-ctx.Done():
			return domain.Response{}, domain.WrapError(domain.CodeTimeout, ctx.Err(), "fake: контекст отменён")
		}
	}
	if f.err != nil {
		return domain.Response{}, f.err
	}
	if f.resp.Provider == "" {
		f.resp.Provider = f.name
	}
	return f.resp, nil
}

func newTestService(p *fakeProvider, timeout time.Duration) *Service {
	r := provider.NewRegistry()
	if p != nil {
		r.Register(p)
		r.SetAliases(map[string]string{"тест": "fake-model"})
	}
	return New(parser.New(), r, timeout)
}

func TestService_Process_Success(t *testing.T) {
	fake := &fakeProvider{
		name:   "fake",
		models: []string{"fake-model"},
		resp:   domain.Response{Model: "fake-model", Provider: "fake", Content: "ответ модели"},
	}
	svc := newTestService(fake, 5*time.Second)

	result, err := svc.Process(context.Background(), "  тест  привет, мир!  ")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if result.Model != "fake-model" || result.Provider != "fake" || result.Answer != "ответ модели" {
		t.Errorf("неожиданный результат: %+v", result)
	}
	if result.ElapsedMS < 0 {
		t.Errorf("ElapsedMS не должен быть отрицательным: %d", result.ElapsedMS)
	}
	if fake.gotReq.Model != "fake-model" {
		t.Errorf("провайдеру ушла модель %q, ожидалась fake-model", fake.gotReq.Model)
	}
	if len(fake.gotReq.Messages) != 1 ||
		fake.gotReq.Messages[0].Role != "user" ||
		fake.gotReq.Messages[0].Content != "привет, мир!" {
		t.Errorf("неожиданные сообщения: %+v", fake.gotReq.Messages)
	}
}

func TestService_Process_ParserErrors(t *testing.T) {
	svc := newTestService(nil, 5*time.Second)

	tests := []struct {
		text string
		want domain.Code
	}{
		{"   ", domain.CodeEmptyText},
		{",,, : !", domain.CodeModelMissing},
		{"тест", domain.CodePromptMissing},
	}
	for _, tt := range tests {
		_, err := svc.Process(context.Background(), tt.text)
		if code := domain.ErrorCode(err); code != tt.want {
			t.Errorf("Process(%q): ожидался код %s, получен %s (%v)", tt.text, tt.want, code, err)
		}
	}
}

func TestService_Process_UnknownModel(t *testing.T) {
	svc := newTestService(&fakeProvider{name: "fake", models: []string{"fake-model"}}, 5*time.Second)
	_, err := svc.Process(context.Background(), "нету привет")
	if code := domain.ErrorCode(err); code != domain.CodeUnknownModel {
		t.Errorf("ожидался код %s, получен %s (%v)", domain.CodeUnknownModel, code, err)
	}
}

func TestService_Process_ProviderErrorPassthrough(t *testing.T) {
	fake := &fakeProvider{
		name:   "fake",
		models: []string{"fake-model"},
		err:    domain.NewError(domain.CodeProviderFailed, "провайдер перегружен"),
	}
	svc := newTestService(fake, 5*time.Second)

	_, err := svc.Process(context.Background(), "тест привет")
	if code := domain.ErrorCode(err); code != domain.CodeProviderFailed {
		t.Errorf("код ошибки провайдера должен сохраниться, получен %s (%v)", code, err)
	}
}

func TestService_Process_ServiceTimeout(t *testing.T) {
	fake := &fakeProvider{name: "fake", models: []string{"fake-model"}, sleep: 300 * time.Millisecond}
	svc := newTestService(fake, 30*time.Millisecond)

	_, err := svc.Process(context.Background(), "тест привет")
	if code := domain.ErrorCode(err); code != domain.CodeTimeout {
		t.Errorf("ожидался код %s, получен %s (%v)", domain.CodeTimeout, code, err)
	}
}

func TestService_Process_ZeroTimeoutMeansNoTimeout(t *testing.T) {
	fake := &fakeProvider{name: "fake", models: []string{"fake-model"}, sleep: 30 * time.Millisecond}
	svc := newTestService(fake, 0)

	if _, err := svc.Process(context.Background(), "тест привет"); err != nil {
		t.Fatalf("медленный запрос должен проходить без таймаута, получено: %v", err)
	}
}

func TestService_ProvidersAndAliases(t *testing.T) {
	fake := &fakeProvider{name: "fake", models: []string{"fake-model"}}
	svc := newTestService(fake, 5*time.Second)

	providers := svc.Providers()
	if len(providers) != 1 || providers[0].Name != "fake" {
		t.Errorf("неожиданный список провайдеров: %+v", providers)
	}
	if aliases := svc.Aliases(); aliases["тест"] != "fake-model" {
		t.Errorf("неожиданные псевдонимы: %+v", aliases)
	}
}
