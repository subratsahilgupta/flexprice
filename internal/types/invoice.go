package types

type InvoiceCadence string

const (
	// InvoiceCadenceArrear raises an invoice at the end of each billing period (in arrears)
	InvoiceCadenceArrear InvoiceCadence = "ARREAR"
	// InvoiceCadenceAdvance raises an invoice at the beginning of each billing period (in advance)
	InvoiceCadenceAdvance InvoiceCadence = "ADVANCE"
)
