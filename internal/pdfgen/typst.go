package pdfgen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
	"time"

	domain "github.com/flexprice/flexprice/internal/domain/pdfgen"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
)

// TypstRenderer handles rendering Typst templates
type TypstRenderer struct {
	log *logger.Logger
}

// NewTypstRenderer creates a new Typst renderer
func NewTypstRenderer(log *logger.Logger) InvoiceRenderer {
	return &TypstRenderer{log: log}
}

// PrepareTemplate converts invoice data to a typst format and prepares a temporary file
func (r *TypstRenderer) PrepareTemplate(templatePath string, data *domain.InvoiceData) (string, error) {
	// Read the template file
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return "", ierr.WithError(err).WithMessage("failed to read template file").Mark(ierr.ErrSystem)
	}

	// get parent directory of template path
	templateDir := filepath.Dir(templatePath)

	// Create the .typ file with the invoice data
	typPath := filepath.Join(templateDir, fmt.Sprintf("invoice-%s.typ", data.ID))

	// Create the typst template with Go's templating
	tmpl, err := template.New("invoice").Parse(string(templateContent))
	if err != nil {
		return "", ierr.WithError(err).WithMessage("failed to parse template").Mark(ierr.ErrSystem)
	}

	f, err := os.Create(typPath)
	if err != nil {
		return "", ierr.WithError(err).WithMessage("failed to create temp file").Mark(ierr.ErrSystem)
	}
	defer f.Close()

	// Convert data to typst-compatible format
	typstData := convertToTypstFormat(data)

	err = tmpl.Execute(f, typstData)
	if err != nil {
		return "", ierr.WithError(err).WithMessage("failed to render template").Mark(ierr.ErrSystem)
	}

	return typPath, nil
}

// CompileTemplate compiles a Typst template into a PDF
func (r *TypstRenderer) CompileTemplate(id, templatePath string, fontDir string) ([]byte, error) {
	// Get the directory of the file
	dir := filepath.Dir(templatePath)
	// clean up the pdf file
	defer func() {
		os.Remove(filepath.Join(dir, fmt.Sprintf("invoice-%s.pdf", id)))
	}()

	// Create the typst compile command
	args := []string{
		"compile",
		templatePath,
	}

	// Add font path if provided
	if fontDir != "" {
		args = append(args, "--font-path", fontDir)
	}

	// find typst binary
	typstBinaryPath, err := exec.LookPath("typst")
	if err != nil {
		return nil, ierr.WithError(err).WithHint("failed to find typst binary").Mark(ierr.ErrSystem)
	}

	r.log.Infof("typst binary path: %s", typstBinaryPath)
	// Execute typst command
	cmd := exec.Command(typstBinaryPath, args...)
	r.log.Infof("typst command: %s", cmd.String())
	_, err = cmd.CombinedOutput()
	if err != nil {
		return nil, ierr.WithError(err).WithHint("failed to compile typst template").Mark(ierr.ErrSystem)
	}

	// Read the generated PDF
	pdfBytes, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("invoice-%s.pdf", id)))
	if err != nil {
		return nil, ierr.WithError(err).WithHint("failed to read compiled PDF").Mark(ierr.ErrSystem)
	}

	return pdfBytes, nil
}

// TypstData represents data in a format suitable for Typst templates
type TypstData struct {
	InvoiceNumber   string      `json:"InvoiceNumber"`
	Title           string      `json:"Title"`
	InvoiceID       string      `json:"InvoiceID"`
	CustomerID      string      `json:"CustomerID"`
	SubscriptionID  string      `json:"SubscriptionID,omitempty"`
	InvoiceType     string      `json:"InvoiceType"`
	InvoiceStatus   string      `json:"InvoiceStatus"`
	PaymentStatus   string      `json:"PaymentStatus"`
	IssuingDate     string      `json:"IssuingDate"`
	DueDate         string      `json:"DueDate"`
	PaidAt          string      `json:"PaidAt,omitempty"`
	VoidedAt        string      `json:"VoidedAt,omitempty"`
	FinalizedAt     string      `json:"FinalizedAt,omitempty"`
	PeriodStart     string      `json:"PeriodStart,omitempty"`
	PeriodEnd       string      `json:"PeriodEnd,omitempty"`
	Notes           string      `json:"Notes"`
	AmountDue       float64     `json:"AmountDue"`
	AmountPaid      float64     `json:"AmountPaid"`
	AmountRemaining float64     `json:"AmountRemaining"`
	VAT             float64     `json:"VAT"`
	BillingReason   string      `json:"BillingReason,omitempty"`
	BannerImage     string      `json:"BannerImage,omitempty"`
	Biller          BillerMap   `json:"Biller"`
	Recipient       BillerMap   `json:"Recipient"`
	Items           []TypstItem `json:"Items"`
}

// BillerMap represents formatted biller information for Typst
type BillerMap map[string]interface{}

// ItemMap represents formatted line item information for Typst
type TypstItem struct {
	PlanDisplayName string  `json:"PlanDisplayName"`
	DisplayName     string  `json:"DisplayName"`
	PeriodStart     string  `json:"PeriodStart,omitempty"`
	PeriodEnd       string  `json:"PeriodEnd,omitempty"`
	Amount          float64 `json:"Amount"`
	Quantity        float64 `json:"Quantity"`
}

// convertToTypstFormat converts from the service data model to Typst-compatible format
func convertToTypstFormat(data *domain.InvoiceData) TypstData {
	// Default title
	title := "Invoice " + data.InvoiceNumber

	// Format dates for Typst
	issuingDate := formatTypstDate(time.Now())
	dueDate := formatTypstDate(data.DueDate)

	// Format optional dates
	periodStart := ""
	if data.PeriodStart != nil {
		periodStart = formatTypstDate(*data.PeriodStart)
	}

	periodEnd := ""
	if data.PeriodEnd != nil {
		periodEnd = formatTypstDate(*data.PeriodEnd)
	}

	// Format biller and recipient as maps
	billerMap := mapFromBiller(data.Biller)
	recipientMap := mapFromRecipient(data.Recipient)

	// Format items
	items := make([]TypstItem, len(data.LineItems))
	for i, item := range data.LineItems {
		items[i] = mapFromLineItem(item)
	}

	// Convert decimal values to float for Typst
	amountDue, _ := data.AmountDue.Float64()
	amountPaid, _ := data.AmountPaid.Float64()
	amountRemaining, _ := data.AmountRemaining.Float64()

	return TypstData{
		InvoiceNumber:   data.InvoiceNumber,
		Title:           title,
		InvoiceID:       data.ID,
		CustomerID:      data.CustomerID,
		SubscriptionID:  data.SubscriptionID,
		InvoiceType:     data.InvoiceType,
		InvoiceStatus:   data.InvoiceStatus,
		PaymentStatus:   data.PaymentStatus,
		IssuingDate:     issuingDate,
		DueDate:         dueDate,
		PeriodStart:     periodStart,
		PeriodEnd:       periodEnd,
		Notes:           data.Notes,
		AmountDue:       amountDue,
		AmountPaid:      amountPaid,
		AmountRemaining: amountRemaining,
		VAT:             data.VAT,
		BillingReason:   data.BillingReason,
		Biller:          billerMap,
		Recipient:       recipientMap,
		Items:           items,
	}
}

// formatTypstDate formats a time.Time in YYYY-MM-DD format for Typst
func formatTypstDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// mapFromBiller converts BillerInfo to a map for Typst template
func mapFromBiller(info *domain.BillerInfo) BillerMap {
	if info == nil {
		return BillerMap{}
	}

	result := BillerMap{
		"name": info.Name,
	}

	// Add optional fields if present
	if info.Email != "" {
		result["email"] = info.Email
	}
	if info.Website != "" {
		result["website"] = info.Website
	}
	if info.HelpEmail != "" {
		result["help-email"] = info.HelpEmail
	}
	if info.PaymentInstructions != "" {
		result["payment-instructions"] = info.PaymentInstructions
	}

	// Add address
	result["address"] = BillerMap{
		"street":      info.Address.Street,
		"city":        info.Address.City,
		"postal-code": info.Address.PostalCode,
	}

	if info.Address.State != "" {
		result["address"].(BillerMap)["state"] = info.Address.State
	}
	if info.Address.Country != "" {
		result["address"].(BillerMap)["country"] = info.Address.Country
	}

	return result
}

// mapFromRecipient converts RecipientInfo to a map for Typst template
func mapFromRecipient(info *domain.RecipientInfo) BillerMap {
	if info == nil {
		return BillerMap{}
	}

	result := BillerMap{
		"name": info.Name,
	}

	if info.Email != "" {
		result["email"] = info.Email
	}

	// Add address
	result["address"] = BillerMap{
		"street":      info.Address.Street,
		"city":        info.Address.City,
		"postal-code": info.Address.PostalCode,
	}

	if info.Address.State != "" {
		result["address"].(BillerMap)["state"] = info.Address.State
	}
	if info.Address.Country != "" {
		result["address"].(BillerMap)["country"] = info.Address.Country
	}

	return result
}

// mapFromLineItem converts LineItemData to a map for Typst template
func mapFromLineItem(item domain.LineItemData) TypstItem {
	amount, _ := item.Amount.Float64()
	quantity, _ := item.Quantity.Float64()

	result := TypstItem{
		PlanDisplayName: item.PlanDisplayName,
		DisplayName:     item.DisplayName,
		Amount:          amount,
		Quantity:        quantity,
	}

	// Add period dates if present
	if item.PeriodStart != nil {
		result.PeriodStart = formatTypstDate(*item.PeriodStart)
	}
	if item.PeriodEnd != nil {
		result.PeriodEnd = formatTypstDate(*item.PeriodEnd)
	}

	return result
}
