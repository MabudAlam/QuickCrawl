package utils

type ErrorCode string

const (
	CodeHttp              ErrorCode = "http_error"
	CodeTargetUnreachable ErrorCode = "target_unreachable"
	CodeInvalidURL        ErrorCode = "invalid_url"
	CodeInvalidRequest    ErrorCode = "invalid_request"
	CodeRendererError     ErrorCode = "renderer_error"
	CodeExtractionErr     ErrorCode = "extraction_error"
	CodeCrawlError        ErrorCode = "crawl_error"
	CodeTimeout           ErrorCode = "timeout"
	CodeConfigError       ErrorCode = "config_error"
	CodeNotFound          ErrorCode = "not_found"
	CodeRateLimited       ErrorCode = "rate_limited"
	CodeInternalErr       ErrorCode = "internal_error"
	CodeForbidden         ErrorCode = "forbidden"
)

type QuickCrawlError struct {
	Message string    `json:"message"`
	Code    ErrorCode `json:"code"`
}

func (e *QuickCrawlError) Error() string {
	return e.Message
}

func (e *QuickCrawlError) GetCode() ErrorCode {
	return e.Code
}

type QuickCrawlErrorFactory struct {
	code ErrorCode
}

func NewErrorFactory(code ErrorCode) QuickCrawlErrorFactory {
	return QuickCrawlErrorFactory{code: code}
}

func (f QuickCrawlErrorFactory) New(message string) *QuickCrawlError {
	return &QuickCrawlError{Message: message, Code: f.code}
}

func (f QuickCrawlErrorFactory) Wrap(err error) *QuickCrawlError {
	if err == nil {
		return nil
	}
	return &QuickCrawlError{Message: err.Error(), Code: f.code}
}

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

func NewQuickCrawlError(message string, code ErrorCode) *QuickCrawlError {
	return &QuickCrawlError{Message: message, Code: code}
}

type QuickCrawlResult[T any] struct {
	Value *T               `json:"value,omitempty"`
	Err   *QuickCrawlError `json:"error,omitempty"`
}

func (r *QuickCrawlResult[T]) Ok(value T) *QuickCrawlResult[T] {
	return &QuickCrawlResult[T]{Value: &value, Err: nil}
}

func (r *QuickCrawlResult[T]) Fail(message string, code ErrorCode) *QuickCrawlResult[T] {
	return &QuickCrawlResult[T]{Value: nil, Err: &QuickCrawlError{Message: message, Code: code}}
}