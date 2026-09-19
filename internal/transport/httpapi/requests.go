package httpapi

type processRequest struct {
	Text string `json:"text"`
}

type chatRequest struct {
	SessionID string `json:"session_id"` // пусто — сервер создаст новую сессию
	Text      string `json:"text"`       // "<модель> <запрос>"
}

type chatEnvelope struct {
	Status    string `json:"status"`
	SessionID string `json:"session_id"`
	Model     string `json:"model"`
	Provider  string `json:"provider"`
	Answer    string `json:"answer"`
	ElapsedMS int64  `json:"elapsed_ms"`
}
