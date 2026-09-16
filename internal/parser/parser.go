// Package parser разбирает текст запроса на имя модели и запрос.
//
// Ожидаемый формат текста:
//
//	<модель> <запрос>
//
// например: "gpt-4o Привет, кто ты?" или "claude-3 расскажи анекдот".
// Прежний формат с ключевым словом тоже принимается для совместимости:
package parser

import (
	"strings"
	"unicode"

	"look-backend/internal/domain"
)

// defaultKeywords — ключевые слова старого формата: если текст начинается
// с одного из них, оно пропускается. Настраивается через LOOK_KEYWORDS.
var defaultKeywords = []string{"start", "старт"}

type Result struct {
	Model  string
	Prompt string
}

// Parser разбирает текст: первое слово — модель, остальное — запрос.
type Parser struct {
	keywords []string // в нижнем регистре
}

// New создаёт Parser; без аргументов используются ключевые слова по умолчанию.
func New(keywords ...string) *Parser {
	if len(keywords) == 0 {
		keywords = defaultKeywords
	}
	lowered := make([]string, len(keywords))
	for i, kw := range keywords {
		lowered[i] = strings.ToLower(kw)
	}
	return &Parser{keywords: lowered}
}

// Keywords возвращает ключевые слова, которые игнорируются перед моделью.
func (p *Parser) Keywords() []string {
	return append([]string(nil), p.keywords...)
}

// Parse берёт первое слово текста как модель, остальное — как запрос.
func (p *Parser) Parse(text string) (Result, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Result{}, domain.NewError(domain.CodeEmptyText, "text обязателен и не может быть пустым")
	}

	// текст состоит только из ключевого слова — модели в нём нет
	lower := strings.ToLower(trimmed)
	for _, kw := range p.keywords {
		if lower == kw {
			return Result{}, domain.NewError(domain.CodeModelMissing,
				`не указана модель; формат: "<модель> <запрос>"`)
		}
	}

	model, prompt := splitModelAndPrompt(p.skipKeyword([]rune(trimmed)))
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

// skipKeyword пропускает ключевое слово старого формата, если текст
// начинается с него: "looking" или "луковый" ключевым словом не считаются.
func (p *Parser) skipKeyword(runes []rune) []rune {
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}
	for _, kw := range p.keywords {
		kwRunes := []rune(kw)
		if len(kwRunes) >= len(runes) {
			continue
		}
		if !equalRunes(lower[:len(kwRunes)], kwRunes) {
			continue
		}
		if !isWordBoundary(runes, 0, len(kwRunes)) {
			continue
		}
		return runes[len(kwRunes):]
	}
	return runes
}

// splitModelAndPrompt отделяет токен модели от запроса. Поддерживаются
// варианты: "gpt-4 привет", "gpt-4: привет", "gpt-4:привет",
// "gpt-4, привет" — знаки препинания сразу после модели отбрасываются.
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
	// "gpt-4:привет" — модель и запрос в одном токене
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

// isWordBoundary проверяет, что слово на [from, to) не примыкает
// вплотную к буквам или цифрам.
func isWordBoundary(runes []rune, from, to int) bool {
	if from > 0 && isWordRune(runes[from-1]) {
		return false
	}
	if to < len(runes) && isWordRune(runes[to]) {
		return false
	}
	return true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func equalRunes(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
