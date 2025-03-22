package pdfgen

import (
	domain "github.com/flexprice/flexprice/internal/domain/pdfgen"
)

type InvoiceRenderer interface {
	PrepareTemplate(templatePath string, data *domain.InvoiceData) (string, error)
	CompileTemplate(id, templatePath, fontDir string) ([]byte, error)
}
