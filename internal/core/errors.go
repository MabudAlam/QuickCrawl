package core

import (
	"fmt"
)

type ErrorCode string

const (
	CodeHttp              ErrorCode = "http_error"
	CodeTargetUnreachable ErrorCode = "target_unreachable"
	CodeInvalidURL        ErrorCode = "invalid_url"
	CodeInvalidRequest    ErrorCode = "invalid_request"
	CodeRendererError     ErrorCode = "renderer_error"
	CodeExtractionError   ErrorCode = "extraction_error"
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

func (e *QuickCrawlError) Code_() ErrorCode {
	return e.Code
}

type ErrorFactory struct {
	code ErrorCode
}

func NewErrorFactory(code ErrorCode) ErrorFactory {
	return ErrorFactory{code: code}
}

func (f ErrorFactory) New(message string) *QuickCrawlError {
	return &QuickCrawlError{Message: message, Code: f.code}
}

func (f ErrorFactory) Wrap(err error) *QuickCrawlError {
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
	ErrRendererError      = NewErrorFactory(CodeRendererError)
	ErrExtraction         = NewErrorFactory(CodeExtractionError)
	ErrCrawl              = NewErrorFactory(CodeCrawlError)
	ErrTimeout            = NewErrorFactory(CodeTimeout)
	ErrConfig             = NewErrorFactory(CodeConfigError)
	ErrBrowserNotAvailable = NewErrorFactory(CodeRendererError)
	ErrNotFound          = NewErrorFactory(CodeNotFound)
	ErrRateLimited       = NewErrorFactory(CodeRateLimited)
	ErrInternal          = NewErrorFactory(CodeInternalErr)
	ErrForbidden         = NewErrorFactory(CodeForbidden)
)

func NewQuickCrawlError(message string, code ErrorCode) *QuickCrawlError {
	return &QuickCrawlError{Message: message, Code: code}
}

func PrefixError(prefix string, err *QuickCrawlError) *QuickCrawlError {
	if err == nil {
		return nil
	}
	return &QuickCrawlError{
		Message: fmt.Sprintf("%s: %s", prefix, err.Message),
		Code:    err.Code,
	}
}
