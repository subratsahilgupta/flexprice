// internal/domain/errors/errors.go
package errors

type AttributeNotFoundError struct {
	Attribute string
}

func (e *AttributeNotFoundError) Error() string {
	return "attribute not found: " + e.Attribute
}

func NewAttributeNotFoundError(attribute string) error {
	return &AttributeNotFoundError{
		Attribute: attribute,
	}
}
