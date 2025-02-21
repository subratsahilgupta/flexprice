package tenant

import (
	ierr "github.com/flexprice/flexprice/internal/errors"
)

// sample domain level error creation

type ErrorCode string

const (
	ErrTenantDefault  ErrorCode = "T0000"
	ErrTenantNotFound ErrorCode = "T0001"
)

type Error struct {
	Code       ErrorCode
	LogMessage string
	Message    string
	Details    map[string]any
	Marker     error
}

func Wrap(err error, t Error) error {
	if err == nil {
		return nil
	}

	if t.Details == nil {
		t.Details = make(map[string]any)
	}

	t.Details["cause"] = err.Error()

	return NewError(t)
}

func NewError(t Error) error {
	if t.Message == "" {
		t.Message = "An unexpected error occurred"
	}

	if t.Marker == nil {
		t.Marker = ierr.ErrSystem
	}

	if t.LogMessage == "" {
		t.LogMessage = t.Message
	}

	if t.Code == "" {
		t.Code = ErrTenantDefault
	}

	return ierr.NewError(t.LogMessage).
		WithHintf("%s - %s", t.Code, t.Message).
		WithReportableDetails(t.Details).
		Mark(t.Marker)
}

func NewTenantNotFoundError(id string) error {
	return ierr.NewError("tenant not found").WithHintf("tenant not found for id: %s", id).Mark(ierr.ErrNotFound)
}

func NewTenantErrorWithDetails(
	logMessage string,
	message string,
	details map[string]any,
) error {
	return ierr.NewError(logMessage).WithHint(message).WithReportableDetails(details).Mark(ierr.ErrInvalidOperation)
}
