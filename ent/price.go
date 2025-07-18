// Code generated by ent, DO NOT EDIT.

package ent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"github.com/flexprice/flexprice/ent/price"
	"github.com/flexprice/flexprice/ent/schema"
)

// Price is the model entity for the Price schema.
type Price struct {
	config `json:"-"`
	// ID of the ent.
	ID string `json:"id,omitempty"`
	// TenantID holds the value of the "tenant_id" field.
	TenantID string `json:"tenant_id,omitempty"`
	// Status holds the value of the "status" field.
	Status string `json:"status,omitempty"`
	// CreatedAt holds the value of the "created_at" field.
	CreatedAt time.Time `json:"created_at,omitempty"`
	// UpdatedAt holds the value of the "updated_at" field.
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	// CreatedBy holds the value of the "created_by" field.
	CreatedBy string `json:"created_by,omitempty"`
	// UpdatedBy holds the value of the "updated_by" field.
	UpdatedBy string `json:"updated_by,omitempty"`
	// EnvironmentID holds the value of the "environment_id" field.
	EnvironmentID string `json:"environment_id,omitempty"`
	// Amount holds the value of the "amount" field.
	Amount float64 `json:"amount,omitempty"`
	// Currency holds the value of the "currency" field.
	Currency string `json:"currency,omitempty"`
	// DisplayAmount holds the value of the "display_amount" field.
	DisplayAmount string `json:"display_amount,omitempty"`
	// PlanID holds the value of the "plan_id" field.
	PlanID string `json:"plan_id,omitempty"`
	// Type holds the value of the "type" field.
	Type string `json:"type,omitempty"`
	// BillingPeriod holds the value of the "billing_period" field.
	BillingPeriod string `json:"billing_period,omitempty"`
	// BillingPeriodCount holds the value of the "billing_period_count" field.
	BillingPeriodCount int `json:"billing_period_count,omitempty"`
	// BillingModel holds the value of the "billing_model" field.
	BillingModel string `json:"billing_model,omitempty"`
	// BillingCadence holds the value of the "billing_cadence" field.
	BillingCadence string `json:"billing_cadence,omitempty"`
	// InvoiceCadence holds the value of the "invoice_cadence" field.
	InvoiceCadence string `json:"invoice_cadence,omitempty"`
	// TrialPeriod holds the value of the "trial_period" field.
	TrialPeriod int `json:"trial_period,omitempty"`
	// MeterID holds the value of the "meter_id" field.
	MeterID *string `json:"meter_id,omitempty"`
	// FilterValues holds the value of the "filter_values" field.
	FilterValues map[string][]string `json:"filter_values,omitempty"`
	// TierMode holds the value of the "tier_mode" field.
	TierMode *string `json:"tier_mode,omitempty"`
	// Tiers holds the value of the "tiers" field.
	Tiers []schema.PriceTier `json:"tiers,omitempty"`
	// TransformQuantity holds the value of the "transform_quantity" field.
	TransformQuantity schema.TransformQuantity `json:"transform_quantity,omitempty"`
	// LookupKey holds the value of the "lookup_key" field.
	LookupKey string `json:"lookup_key,omitempty"`
	// Description holds the value of the "description" field.
	Description string `json:"description,omitempty"`
	// Metadata holds the value of the "metadata" field.
	Metadata map[string]string `json:"metadata,omitempty"`
	// Edges holds the relations/edges for other nodes in the graph.
	// The values are being populated by the PriceQuery when eager-loading is set.
	Edges        PriceEdges `json:"edges"`
	selectValues sql.SelectValues
}

// PriceEdges holds the relations/edges for other nodes in the graph.
type PriceEdges struct {
	// Costsheet holds the value of the costsheet edge.
	Costsheet []*Costsheet `json:"costsheet,omitempty"`
	// loadedTypes holds the information for reporting if a
	// type was loaded (or requested) in eager-loading or not.
	loadedTypes [1]bool
}

// CostsheetOrErr returns the Costsheet value or an error if the edge
// was not loaded in eager-loading.
func (e PriceEdges) CostsheetOrErr() ([]*Costsheet, error) {
	if e.loadedTypes[0] {
		return e.Costsheet, nil
	}
	return nil, &NotLoadedError{edge: "costsheet"}
}

// scanValues returns the types for scanning values from sql.Rows.
func (*Price) scanValues(columns []string) ([]any, error) {
	values := make([]any, len(columns))
	for i := range columns {
		switch columns[i] {
		case price.FieldFilterValues, price.FieldTiers, price.FieldTransformQuantity, price.FieldMetadata:
			values[i] = new([]byte)
		case price.FieldAmount:
			values[i] = new(sql.NullFloat64)
		case price.FieldBillingPeriodCount, price.FieldTrialPeriod:
			values[i] = new(sql.NullInt64)
		case price.FieldID, price.FieldTenantID, price.FieldStatus, price.FieldCreatedBy, price.FieldUpdatedBy, price.FieldEnvironmentID, price.FieldCurrency, price.FieldDisplayAmount, price.FieldPlanID, price.FieldType, price.FieldBillingPeriod, price.FieldBillingModel, price.FieldBillingCadence, price.FieldInvoiceCadence, price.FieldMeterID, price.FieldTierMode, price.FieldLookupKey, price.FieldDescription:
			values[i] = new(sql.NullString)
		case price.FieldCreatedAt, price.FieldUpdatedAt:
			values[i] = new(sql.NullTime)
		default:
			values[i] = new(sql.UnknownType)
		}
	}
	return values, nil
}

// assignValues assigns the values that were returned from sql.Rows (after scanning)
// to the Price fields.
func (pr *Price) assignValues(columns []string, values []any) error {
	if m, n := len(values), len(columns); m < n {
		return fmt.Errorf("mismatch number of scan values: %d != %d", m, n)
	}
	for i := range columns {
		switch columns[i] {
		case price.FieldID:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field id", values[i])
			} else if value.Valid {
				pr.ID = value.String
			}
		case price.FieldTenantID:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field tenant_id", values[i])
			} else if value.Valid {
				pr.TenantID = value.String
			}
		case price.FieldStatus:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field status", values[i])
			} else if value.Valid {
				pr.Status = value.String
			}
		case price.FieldCreatedAt:
			if value, ok := values[i].(*sql.NullTime); !ok {
				return fmt.Errorf("unexpected type %T for field created_at", values[i])
			} else if value.Valid {
				pr.CreatedAt = value.Time
			}
		case price.FieldUpdatedAt:
			if value, ok := values[i].(*sql.NullTime); !ok {
				return fmt.Errorf("unexpected type %T for field updated_at", values[i])
			} else if value.Valid {
				pr.UpdatedAt = value.Time
			}
		case price.FieldCreatedBy:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field created_by", values[i])
			} else if value.Valid {
				pr.CreatedBy = value.String
			}
		case price.FieldUpdatedBy:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field updated_by", values[i])
			} else if value.Valid {
				pr.UpdatedBy = value.String
			}
		case price.FieldEnvironmentID:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field environment_id", values[i])
			} else if value.Valid {
				pr.EnvironmentID = value.String
			}
		case price.FieldAmount:
			if value, ok := values[i].(*sql.NullFloat64); !ok {
				return fmt.Errorf("unexpected type %T for field amount", values[i])
			} else if value.Valid {
				pr.Amount = value.Float64
			}
		case price.FieldCurrency:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field currency", values[i])
			} else if value.Valid {
				pr.Currency = value.String
			}
		case price.FieldDisplayAmount:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field display_amount", values[i])
			} else if value.Valid {
				pr.DisplayAmount = value.String
			}
		case price.FieldPlanID:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field plan_id", values[i])
			} else if value.Valid {
				pr.PlanID = value.String
			}
		case price.FieldType:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field type", values[i])
			} else if value.Valid {
				pr.Type = value.String
			}
		case price.FieldBillingPeriod:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field billing_period", values[i])
			} else if value.Valid {
				pr.BillingPeriod = value.String
			}
		case price.FieldBillingPeriodCount:
			if value, ok := values[i].(*sql.NullInt64); !ok {
				return fmt.Errorf("unexpected type %T for field billing_period_count", values[i])
			} else if value.Valid {
				pr.BillingPeriodCount = int(value.Int64)
			}
		case price.FieldBillingModel:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field billing_model", values[i])
			} else if value.Valid {
				pr.BillingModel = value.String
			}
		case price.FieldBillingCadence:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field billing_cadence", values[i])
			} else if value.Valid {
				pr.BillingCadence = value.String
			}
		case price.FieldInvoiceCadence:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field invoice_cadence", values[i])
			} else if value.Valid {
				pr.InvoiceCadence = value.String
			}
		case price.FieldTrialPeriod:
			if value, ok := values[i].(*sql.NullInt64); !ok {
				return fmt.Errorf("unexpected type %T for field trial_period", values[i])
			} else if value.Valid {
				pr.TrialPeriod = int(value.Int64)
			}
		case price.FieldMeterID:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field meter_id", values[i])
			} else if value.Valid {
				pr.MeterID = new(string)
				*pr.MeterID = value.String
			}
		case price.FieldFilterValues:
			if value, ok := values[i].(*[]byte); !ok {
				return fmt.Errorf("unexpected type %T for field filter_values", values[i])
			} else if value != nil && len(*value) > 0 {
				if err := json.Unmarshal(*value, &pr.FilterValues); err != nil {
					return fmt.Errorf("unmarshal field filter_values: %w", err)
				}
			}
		case price.FieldTierMode:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field tier_mode", values[i])
			} else if value.Valid {
				pr.TierMode = new(string)
				*pr.TierMode = value.String
			}
		case price.FieldTiers:
			if value, ok := values[i].(*[]byte); !ok {
				return fmt.Errorf("unexpected type %T for field tiers", values[i])
			} else if value != nil && len(*value) > 0 {
				if err := json.Unmarshal(*value, &pr.Tiers); err != nil {
					return fmt.Errorf("unmarshal field tiers: %w", err)
				}
			}
		case price.FieldTransformQuantity:
			if value, ok := values[i].(*[]byte); !ok {
				return fmt.Errorf("unexpected type %T for field transform_quantity", values[i])
			} else if value != nil && len(*value) > 0 {
				if err := json.Unmarshal(*value, &pr.TransformQuantity); err != nil {
					return fmt.Errorf("unmarshal field transform_quantity: %w", err)
				}
			}
		case price.FieldLookupKey:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field lookup_key", values[i])
			} else if value.Valid {
				pr.LookupKey = value.String
			}
		case price.FieldDescription:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field description", values[i])
			} else if value.Valid {
				pr.Description = value.String
			}
		case price.FieldMetadata:
			if value, ok := values[i].(*[]byte); !ok {
				return fmt.Errorf("unexpected type %T for field metadata", values[i])
			} else if value != nil && len(*value) > 0 {
				if err := json.Unmarshal(*value, &pr.Metadata); err != nil {
					return fmt.Errorf("unmarshal field metadata: %w", err)
				}
			}
		default:
			pr.selectValues.Set(columns[i], values[i])
		}
	}
	return nil
}

// Value returns the ent.Value that was dynamically selected and assigned to the Price.
// This includes values selected through modifiers, order, etc.
func (pr *Price) Value(name string) (ent.Value, error) {
	return pr.selectValues.Get(name)
}

// QueryCostsheet queries the "costsheet" edge of the Price entity.
func (pr *Price) QueryCostsheet() *CostsheetQuery {
	return NewPriceClient(pr.config).QueryCostsheet(pr)
}

// Update returns a builder for updating this Price.
// Note that you need to call Price.Unwrap() before calling this method if this Price
// was returned from a transaction, and the transaction was committed or rolled back.
func (pr *Price) Update() *PriceUpdateOne {
	return NewPriceClient(pr.config).UpdateOne(pr)
}

// Unwrap unwraps the Price entity that was returned from a transaction after it was closed,
// so that all future queries will be executed through the driver which created the transaction.
func (pr *Price) Unwrap() *Price {
	_tx, ok := pr.config.driver.(*txDriver)
	if !ok {
		panic("ent: Price is not a transactional entity")
	}
	pr.config.driver = _tx.drv
	return pr
}

// String implements the fmt.Stringer.
func (pr *Price) String() string {
	var builder strings.Builder
	builder.WriteString("Price(")
	builder.WriteString(fmt.Sprintf("id=%v, ", pr.ID))
	builder.WriteString("tenant_id=")
	builder.WriteString(pr.TenantID)
	builder.WriteString(", ")
	builder.WriteString("status=")
	builder.WriteString(pr.Status)
	builder.WriteString(", ")
	builder.WriteString("created_at=")
	builder.WriteString(pr.CreatedAt.Format(time.ANSIC))
	builder.WriteString(", ")
	builder.WriteString("updated_at=")
	builder.WriteString(pr.UpdatedAt.Format(time.ANSIC))
	builder.WriteString(", ")
	builder.WriteString("created_by=")
	builder.WriteString(pr.CreatedBy)
	builder.WriteString(", ")
	builder.WriteString("updated_by=")
	builder.WriteString(pr.UpdatedBy)
	builder.WriteString(", ")
	builder.WriteString("environment_id=")
	builder.WriteString(pr.EnvironmentID)
	builder.WriteString(", ")
	builder.WriteString("amount=")
	builder.WriteString(fmt.Sprintf("%v", pr.Amount))
	builder.WriteString(", ")
	builder.WriteString("currency=")
	builder.WriteString(pr.Currency)
	builder.WriteString(", ")
	builder.WriteString("display_amount=")
	builder.WriteString(pr.DisplayAmount)
	builder.WriteString(", ")
	builder.WriteString("plan_id=")
	builder.WriteString(pr.PlanID)
	builder.WriteString(", ")
	builder.WriteString("type=")
	builder.WriteString(pr.Type)
	builder.WriteString(", ")
	builder.WriteString("billing_period=")
	builder.WriteString(pr.BillingPeriod)
	builder.WriteString(", ")
	builder.WriteString("billing_period_count=")
	builder.WriteString(fmt.Sprintf("%v", pr.BillingPeriodCount))
	builder.WriteString(", ")
	builder.WriteString("billing_model=")
	builder.WriteString(pr.BillingModel)
	builder.WriteString(", ")
	builder.WriteString("billing_cadence=")
	builder.WriteString(pr.BillingCadence)
	builder.WriteString(", ")
	builder.WriteString("invoice_cadence=")
	builder.WriteString(pr.InvoiceCadence)
	builder.WriteString(", ")
	builder.WriteString("trial_period=")
	builder.WriteString(fmt.Sprintf("%v", pr.TrialPeriod))
	builder.WriteString(", ")
	if v := pr.MeterID; v != nil {
		builder.WriteString("meter_id=")
		builder.WriteString(*v)
	}
	builder.WriteString(", ")
	builder.WriteString("filter_values=")
	builder.WriteString(fmt.Sprintf("%v", pr.FilterValues))
	builder.WriteString(", ")
	if v := pr.TierMode; v != nil {
		builder.WriteString("tier_mode=")
		builder.WriteString(*v)
	}
	builder.WriteString(", ")
	builder.WriteString("tiers=")
	builder.WriteString(fmt.Sprintf("%v", pr.Tiers))
	builder.WriteString(", ")
	builder.WriteString("transform_quantity=")
	builder.WriteString(fmt.Sprintf("%v", pr.TransformQuantity))
	builder.WriteString(", ")
	builder.WriteString("lookup_key=")
	builder.WriteString(pr.LookupKey)
	builder.WriteString(", ")
	builder.WriteString("description=")
	builder.WriteString(pr.Description)
	builder.WriteString(", ")
	builder.WriteString("metadata=")
	builder.WriteString(fmt.Sprintf("%v", pr.Metadata))
	builder.WriteByte(')')
	return builder.String()
}

// Prices is a parsable slice of Price.
type Prices []*Price
