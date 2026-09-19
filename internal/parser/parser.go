package parser

import (
	"strings"
	"unicode"

	"look-backend/internal/domain"
)

type Result struct {
	Model  string
	Prompt string
}

// Parser разбирает текст: первое слово — модель, остальное — запрос.
type Parser struct{}

// New создаёт Parser.
func New() *Parser { return &Parser{} }

// Parse берёт первое слово текста как модель, остальное — как запрос.
func (p *Parser) Parse(text string) (Result, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Result{}, domain.NewError(domain.CodeEmptyText, "text обязателен и не может быть пустым")
	}

	model, prompt := splitModelAndPrompt([]rune(trimmed))
	if model == "" {
		return Result{}, domain.NewError(domain.CodeModelMissing,
			`не указана модель; формат: "<модель> <запрос>"`)
	}
	if prompt == "" {
		return Result{}, domain.NewError(domain.CodePromptMissing,
			`после модели не указан запрос; формат: "<модель> <запрос>"`)
	}
	return Result{Model: model, Prompt: prompt}, nil
}

func splitModelAndPrompt(runes []rune) (string, string) {
	i := 0
	for i < len(runes) && isSeparatorRune(runes[i]) {
		i++
	}
	start := i
	for i < len(runes) && !unicode.IsSpace(runes[i]) {
		i++
	}
	token := string(runes[start:i])
	tail := string(runes[i:])
	if token == "" {
		return "", ""
	}

	if before, after, found := strings.Cut(token, ":"); found {
		token, tail = before, after+tail
	}
	token = strings.TrimRight(token, ",;:.?!")
	prompt := strings.TrimSpace(tail)
	prompt = strings.TrimPrefix(prompt, ":")
	return token, strings.TrimSpace(prompt)
}

func isSeparatorRune(r rune) bool {
	return unicode.IsSpace(r) || r == ':' || r == ',' || r == ';' || r == '.' || r == '?' || r == '!'
}
