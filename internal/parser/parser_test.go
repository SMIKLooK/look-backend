package parser

import (
	"testing"

	"look-backend/internal/domain"
)

func TestParser_Parse(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantModel  string
		wantPrompt string
		wantCode   domain.Code // пустая строка — ошибка не ожидается
	}{
		{
			name:       "модель + запрос",
			text:       "gpt-4 Привет, как дела?",
			wantModel:  "gpt-4",
			wantPrompt: "Привет, как дела?",
		},
		{
			name:       "модель с вендором и запрос",
			text:       "google/gemini-3.8-flash расскажи анекдот",
			wantModel:  "google/gemini-3.8-flash",
			wantPrompt: "расскажи анекдот",
		},
		{
			name:       "регистр модели сохраняется",
			text:       "GPT-4 привет",
			wantModel:  "GPT-4",
			wantPrompt: "привет",
		},
		{
			name:       "модель и запрос через двоеточие",
			text:       "gpt-4: привет",
			wantModel:  "gpt-4",
			wantPrompt: "привет",
		},
		{
			name:       "модель и запрос слитно через двоеточие",
			text:       "gpt-4:привет",
			wantModel:  "gpt-4",
			wantPrompt: "привет",
		},
		{
			name:       "запятая после модели",
			text:       "gpt-4, переведи текст",
			wantModel:  "gpt-4",
			wantPrompt: "переведи текст",
		},
		{
			name:       "вопросительный знак после модели",
			text:       "gpt-4? привет",
			wantModel:  "gpt-4",
			wantPrompt: "привет",
		},
		{
			name:       "многострочный запрос",
			text:       "gpt-4 строка1\nстрока2",
			wantModel:  "gpt-4",
			wantPrompt: "строка1\nстрока2",
		},
		{
			name:       "первое слово — модель, даже если это не модель",
			text:       "просто привет",
			wantModel:  "просто",
			wantPrompt: "привет",
		},
		{
			name:       "любое первое слово трактуется как модель",
			text:       "looking good",
			wantModel:  "looking",
			wantPrompt: "good",
		},
		{
			name:     "пустой текст",
			text:     "   ",
			wantCode: domain.CodeEmptyText,
		},
		{
			name:     "одно слово — модель без запроса",
			text:     "look",
			wantCode: domain.CodePromptMissing,
		},
		{
			name:     "нет запроса",
			text:     "gpt-4",
			wantCode: domain.CodePromptMissing,
		},
	}

	p := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Parse(tt.text)
			if tt.wantCode != "" {
				if err == nil {
					t.Fatalf("ожидалась ошибка %s, получен результат %+v", tt.wantCode, got)
				}
				if code := domain.ErrorCode(err); code != tt.wantCode {
					t.Fatalf("ожидался код %s, получен %s (%v)", tt.wantCode, code, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if got.Model != tt.wantModel {
				t.Errorf("model: ожидалось %q, получено %q", tt.wantModel, got.Model)
			}
			if got.Prompt != tt.wantPrompt {
				t.Errorf("prompt: ожидалось %q, получено %q", tt.wantPrompt, got.Prompt)
			}
		})
	}
}
