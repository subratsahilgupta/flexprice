package pdf

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/domain/pdf"
	"github.com/flexprice/flexprice/internal/typst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mocking the typst.Compiler for testing
type MockCompiler struct {
	mock.Mock
}

func (m *MockCompiler) CompileTemplate(templateName string, jsonData []byte, options ...typst.CompileOptsBuilder) ([]byte, error) {
	args := m.Called(templateName, jsonData, options)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockCompiler) CleanupGeneratedFiles(files ...string) {
	m.Called(files)
}

func (m *MockCompiler) Compile(opts typst.CompileOpts) (string, error) {
	args := m.Called(opts)
	return args.String(0), args.Error(1)
}

func (m *MockCompiler) CompileToBytes(opts typst.CompileOpts) ([]byte, error) {
	args := m.Called(opts)
	return args.Get(0).([]byte), args.Error(1)
}

// Test for RenderInvoicePdf
func TestRenderInvoicePdf(t *testing.T) {
	mockCompiler := new(MockCompiler)
	service := &service{
		typst: mockCompiler,
	}

	data := &pdf.InvoiceData{ID: "123"}
	expectedPDF := []byte("mocked PDF content")

	mockCompiler.On("CompileTemplate", "invoice.typ", mock.Anything, mock.Anything).Return(expectedPDF, nil)

	pdf, err := service.RenderInvoicePdf(context.Background(), data)

	assert.NoError(t, err)
	assert.Equal(t, expectedPDF, pdf)
}
