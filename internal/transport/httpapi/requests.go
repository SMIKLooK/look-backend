package httpapi

// processRequest — тело запроса POST /api/v1/process.
type processRequest struct {
	Text string `json:"text"`
}
