package types

import "testing"

const (
	validGSTIN = "27AAMCM4148E1ZD" // PAN AAMCM4148E, state 27
	validPAN   = "AAMCM4148E"
)

func TestTaxMetadataValidate(t *testing.T) {
	tests := []struct {
		name    string
		meta    map[string]string
		wantErr bool
	}{
		{"empty map", map[string]string{}, false},
		{"nil map", nil, false},
		{"unrelated keys only", map[string]string{"foo": "bar"}, false},
		{"valid gstin", map[string]string{MetadataKeyGSTIN: validGSTIN}, false},
		{"valid gstin lowercase is normalised", map[string]string{MetadataKeyGSTIN: "27aamcm4148e1zd"}, false},
		{"valid gstin with surrounding space", map[string]string{MetadataKeyGSTIN: "  " + validGSTIN + " "}, false},
		{"gstin too short", map[string]string{MetadataKeyGSTIN: "27AAMCM4148E1Z"}, true},
		{"gstin missing Z at position 14", map[string]string{MetadataKeyGSTIN: "27AAMCM4148E1AD"}, true},
		{"gstin with bad state code", map[string]string{MetadataKeyGSTIN: "AAAAMCM4148E1ZD"}, true},
		{"valid pan alone", map[string]string{MetadataKeyPAN: validPAN}, false},
		{"pan too short", map[string]string{MetadataKeyPAN: "AAMCM4148"}, true},
		{"pan with digits in letter positions", map[string]string{MetadataKeyPAN: "AAMC14148E"}, true},
		{"gstin and matching pan", map[string]string{MetadataKeyGSTIN: validGSTIN, MetadataKeyPAN: validPAN}, false},
		{"gstin and contradicting pan", map[string]string{MetadataKeyGSTIN: validGSTIN, MetadataKeyPAN: "ZZZZZ1111Z"}, true},
		{"hsn 4 digits", map[string]string{MetadataKeyHSNSAC: "9984"}, false},
		{"hsn 6 digits", map[string]string{MetadataKeyHSNSAC: "998415"}, false},
		{"hsn 8 digits", map[string]string{MetadataKeyHSNSAC: "99841500"}, false},
		{"hsn 5 digits", map[string]string{MetadataKeyHSNSAC: "99841"}, true},
		{"hsn non numeric", map[string]string{MetadataKeyHSNSAC: "99AB15"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTaxMetadata(tt.meta)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestTaxMetadataDerivation(t *testing.T) {
	tests := []struct {
		name              string
		meta              map[string]string
		wantPAN           string
		wantPlaceOfSupply string
	}{
		{
			name:              "derived from gstin when absent",
			meta:              map[string]string{MetadataKeyGSTIN: validGSTIN},
			wantPAN:           validPAN,
			wantPlaceOfSupply: "27",
		},
		{
			name: "explicit values win over derivation",
			meta: map[string]string{
				MetadataKeyGSTIN:         validGSTIN,
				MetadataKeyPAN:           validPAN,
				MetadataKeyPlaceOfSupply: "29",
			},
			wantPAN:           validPAN,
			wantPlaceOfSupply: "29",
		},
		{
			name:              "no derivation from an invalid gstin",
			meta:              map[string]string{MetadataKeyGSTIN: "not-a-gstin"},
			wantPAN:           "",
			wantPlaceOfSupply: "",
		},
		{
			name:              "pan alone does not yield place of supply",
			meta:              map[string]string{MetadataKeyPAN: validPAN},
			wantPAN:           validPAN,
			wantPlaceOfSupply: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := TaxMetadataFromMap(tt.meta)
			if got := tm.PAN(); got != tt.wantPAN {
				t.Errorf("PAN() = %q, want %q", got, tt.wantPAN)
			}
			if got := tm.PlaceOfSupply(); got != tt.wantPlaceOfSupply {
				t.Errorf("PlaceOfSupply() = %q, want %q", got, tt.wantPlaceOfSupply)
			}
		})
	}
}

func TestTaxMetadataNilSafety(t *testing.T) {
	var tm *TaxMetadata
	if tm.GSTIN() != "" || tm.PAN() != "" || tm.PlaceOfSupply() != "" || tm.HSNSAC() != "" {
		t.Error("nil TaxMetadata accessors must return empty strings")
	}
	if tm.ShippingAddress() != nil {
		t.Error("nil TaxMetadata must return a nil shipping address")
	}
	if err := tm.Validate(); err != nil {
		t.Errorf("nil TaxMetadata must validate cleanly, got %v", err)
	}

	var a *Address
	if a.Line1() != "" || a.City() != "" || a.Country() != "" {
		t.Error("nil Address accessors must return empty strings")
	}
	if !a.IsEmpty() {
		t.Error("nil Address must report IsZero")
	}
}

func TestTaxMetadataAbsentKeys(t *testing.T) {
	tm := TaxMetadataFromMap(map[string]string{"unrelated": "value"})
	if tm.GSTIN() != "" || tm.PAN() != "" || tm.HSNSAC() != "" {
		t.Error("absent keys must yield empty strings")
	}
	if tm.ShippingAddress() != nil {
		t.Error("absent shipping keys must yield a nil address")
	}
}

func TestShippingAddressParsing(t *testing.T) {
	t.Run("partial address is still returned", func(t *testing.T) {
		tm := TaxMetadataFromMap(map[string]string{
			MetadataKeyShippingPrefix + "city":    "Mumbai",
			MetadataKeyShippingPrefix + "country": "IN",
		})
		addr := tm.ShippingAddress()
		if addr == nil {
			t.Fatal("expected an address")
		}
		if addr.City() != "Mumbai" || addr.Country() != "IN" {
			t.Errorf("unexpected address: city=%q country=%q", addr.City(), addr.Country())
		}
		if addr.Line1() != "" {
			t.Errorf("Line1() = %q, want empty", addr.Line1())
		}
	})

	t.Run("all fields", func(t *testing.T) {
		tm := TaxMetadataFromMap(map[string]string{
			MetadataKeyShippingPrefix + "line1":       "Ground Floor, B1-002-B",
			MetadataKeyShippingPrefix + "line2":       "Boomerang Building",
			MetadataKeyShippingPrefix + "city":        "Mumbai",
			MetadataKeyShippingPrefix + "state":       "Maharashtra",
			MetadataKeyShippingPrefix + "postal_code": "400072",
			MetadataKeyShippingPrefix + "country":     "IN",
		})
		addr := tm.ShippingAddress()
		if addr == nil {
			t.Fatal("expected an address")
		}
		if addr.Line2() != "Boomerang Building" || addr.State() != "Maharashtra" || addr.PostalCode() != "400072" {
			t.Errorf("unexpected address: %+v", addr)
		}
	})

	t.Run("blank values yield no address", func(t *testing.T) {
		tm := TaxMetadataFromMap(map[string]string{
			MetadataKeyShippingPrefix + "city": "   ",
		})
		if tm.ShippingAddress() != nil {
			t.Error("whitespace-only values must not produce an address")
		}
	})
}
