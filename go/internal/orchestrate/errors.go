package orchestrate

// ErrorCode is a stable machine-readable failure class for sc_ask.
type ErrorCode string

const (
	ErrInvalidInput    ErrorCode = "invalid_input"
	ErrDatabaseMissing ErrorCode = "database_missing"
	ErrNoMatch         ErrorCode = "no_match"
	ErrRouteFailed     ErrorCode = "route_failed"
	ErrToolFailed      ErrorCode = "tool_failed"
	ErrRetryExhausted  ErrorCode = "retry_exhausted"
	ErrStepLimit       ErrorCode = "step_limit"
	ErrInternal        ErrorCode = "internal"
)

// AskError is returned when Ask cannot complete normally.
type AskError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *AskError) Error() string {
	if e == nil {
		return ""
	}
	return string(e.Code) + ": " + e.Message
}

func askErr(code ErrorCode, msg string) *AskError {
	return &AskError{Code: code, Message: msg}
}
