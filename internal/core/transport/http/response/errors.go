package core_http_response

// ErrorResponse is the JSON representation returned for failed HTTP requests.
type ErrorResponse struct {
	Error   string `json:"error"   example:"full error text"`
	Message string `json:"message" example:"short human-readable message"`
}
