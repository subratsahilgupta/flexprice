package service

import (
	"context"
	"os"
	"path/filepath"

	domain "github.com/flexprice/flexprice/internal/domain/pdf"
	"github.com/flexprice/flexprice/internal/pdfgen"
)

// PdfGenService defines the interface for PDF generation operations
type PdfGenService interface {
	// GenerateInvoicePDF generates a PDF for the given invoice ID
	GenerateInvoicePDF(ctx context.Context, invoiceID string) ([]byte, error)

	// RenderInvoice renders an invoice template with the provided data
	RenderInvoice(ctx context.Context, data *domain.InvoiceData) ([]byte, error)

	// GetInvoiceData retrieves invoice data for PDF generation
	GetInvoiceData(ctx context.Context, invoiceID string) (*domain.InvoiceData, error)
}

type PdfGenConfig struct {
	TemplateDir     string
	OutputDir       string
	FontDir         string
	DefaultTemplate string
}

type pdfGenService struct {
	templateDir     string
	outputDir       string
	fontDir         string
	defaultTemplate string
	renderer        pdfgen.InvoiceRenderer
}

func NewPdfGenService(params ServiceParams, renderer pdfgen.InvoiceRenderer) PdfGenService {
	return &pdfGenService{
		templateDir:     "assets/typsts",
		outputDir:       "assets/typsts",
		fontDir:         "assets/fonts",
		defaultTemplate: "invoice.typ",
		renderer:        renderer,
	}
}

// GenerateInvoicePDF implements PdfGenService.
func (s *pdfGenService) GenerateInvoicePDF(ctx context.Context, invoiceID string) ([]byte, error) {
	// Get invoice data
	data, err := s.GetInvoiceData(ctx, invoiceID)
	if err != nil {
		return nil, err
	}

	// Render the PDF
	return s.RenderInvoice(ctx, data)
}

// GetInvoiceData retrieves invoice data for PDF generation
func (s *pdfGenService) GetInvoiceData(ctx context.Context, invoiceID string) (*domain.InvoiceData, error) {
	return nil, nil
}

// RenderInvoice implements PdfGenService.
func (s *pdfGenService) RenderInvoice(ctx context.Context, data *domain.InvoiceData) ([]byte, error) {
	fillerPath := filepath.Join(s.templateDir, "invoice.typ") // Use invoice.typ as the filler template

	// Generate a temporary file with template data filled in
	tmpFile, err := s.renderer.PrepareTemplate(fillerPath, data)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)

	// Compile the default template to PDF
	// Uses default.typ for rendering
	pdfBytes, err := s.renderer.CompileTemplate(
		data.ID,
		tmpFile,
		s.fontDir,
	)
	if err != nil {
		return nil, err
	}

	return pdfBytes, nil
}
