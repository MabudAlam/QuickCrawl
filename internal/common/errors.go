// Package common provides shared utilities and types used across all modules.
// It contains error types, URL validation, constants, and helper functions.
package common

// ErrorCode represents a categorized error code for the application.
// Each error code maps to a specific failure category.
type ErrorCode string

// Application error codes used throughout the codebase.
const (
	CodeHttp              ErrorCode = "http_error"         // HTTP request failed
	CodeTargetUnreachable ErrorCode = "target_unreachable" // Cannot reach the target host
	CodeInvalidURL        ErrorCode = "invalid_url"        // URL format is invalid
	CodeInvalidRequest    ErrorCode = "invalid_request"    // Request parameters are invalid
	CodeRendererError     ErrorCode = "renderer_error"     // Browser rendering failed
	CodeExtractionErr     ErrorCode = "extraction_error"   // Content extraction failed
	CodeCrawlError        ErrorCode = "crawl_error"        // Crawling operation failed
	CodeTimeout           ErrorCode = "timeout"            // Operation timed out
	CodeConfigError       ErrorCode = "config_error"       // Configuration is invalid
	CodeNotFound          ErrorCode = "not_found"          // Resource not found
	CodeRateLimited       ErrorCode = "rate_limited"       // Rate limit exceeded
	CodeInternalErr       ErrorCode = "internal_error"     // Internal server error
	CodeForbidden         ErrorCode = "forbidden"          // Access forbidden by robots.txt
)

// QuickcrawlError represents an error with a code and message.
// It implements the error interface.
type QuickcrawlError struct {
	Message string    `json:"message"` // Human-readable error message
	Code    ErrorCode `json:"code"`    // Machine-readable error code
}

// Error returns the error message (implements error interface).
func (e *QuickcrawlError) Error() string {
	return e.Message
}

// ErrorCode returns the error code.
func (e *QuickcrawlError) ErrorCode() ErrorCode {
	return e.Code
}

// QuickcrawlErrorFactory creates errors with a consistent error code.
// Use it to create errors that share the same error code.
type QuickcrawlErrorFactory struct {
	code ErrorCode
}

// NewErrorFactory creates a new error factory with the given error code.
func NewErrorFactory(code ErrorCode) QuickcrawlErrorFactory {
	return QuickcrawlErrorFactory{code: code}
}

// New creates a new error with the factory's error code and the given message.
func (f QuickcrawlErrorFactory) New(message string) *QuickcrawlError {
	return &QuickcrawlError{Message: message, Code: f.code}
}

// Wrap converts an existing error into a QuickcrawlError with the factory's error code.
// Returns nil if the input error is nil.
func (f QuickcrawlErrorFactory) Wrap(err error) *QuickcrawlError {
	if err == nil {
		return nil
	}
	return &QuickcrawlError{Message: err.Error(), Code: f.code}
}

// Pre-configured error factories for common error types.
// Usage: ErrInvalidURL.New("missing schema")
var (
	ErrHttp              = NewErrorFactory(CodeHttp)
	ErrTargetUnreachable = NewErrorFactory(CodeTargetUnreachable)
	ErrInvalidURL        = NewErrorFactory(CodeInvalidURL)
	ErrInvalidRequest    = NewErrorFactory(CodeInvalidRequest)
	ErrRendererError     = NewErrorFactory(CodeRendererError)
	ErrExtraction        = NewErrorFactory(CodeExtractionErr)
	ErrCrawl             = NewErrorFactory(CodeCrawlError)
	ErrTimeout           = NewErrorFactory(CodeTimeout)
	ErrConfig            = NewErrorFactory(CodeConfigError)
	ErrNotFound          = NewErrorFactory(CodeNotFound)
	ErrRateLimited       = NewErrorFactory(CodeRateLimited)
	ErrInternal          = NewErrorFactory(CodeInternalErr)
	ErrForbidden         = NewErrorFactory(CodeForbidden)
)

// NewQuickcrawlError creates a new error with the given message and error code.
func NewQuickcrawlError(message string, code ErrorCode) *QuickcrawlError {
	return &QuickcrawlError{Message: message, Code: code}
}

// QuickcrawlResult is a generic result type that represents either a value or an error.
// Use it to explicitly handle both success and failure cases.
type QuickcrawlResult[T any] struct {
	Value *T                 `json:"value,omitempty"` // The result value (nil on error)
	Err   *QuickcrawlError `json:"error,omitempty"` // Error if operation failed
}

// Ok returns a successful result containing the given value.
func (r *QuickcrawlResult[T]) Ok(value T) *QuickcrawlResult[T] {
	return &QuickcrawlResult[T]{Value: &value, Err: nil}
}

// Fail returns an error result with the given message and error code.
func (r *QuickcrawlResult[T]) Fail(message string, code ErrorCode) *QuickcrawlResult[T] {
	return &QuickcrawlResult[T]{Value: nil, Err: &QuickcrawlError{Message: message, Code: code}}
}
