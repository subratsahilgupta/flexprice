package service

import (
	"testing"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateAnalyticsGroupBy_ExternalCustomerID is a focused test for the
// Change-2 additive extension: "external_customer_id" is now an accepted
// detailed group_by dimension, alongside the pre-existing meter_id / source /
// properties.* entries (which must keep working unchanged).
func TestValidateAnalyticsGroupBy_ExternalCustomerID(t *testing.T) {
	require.NoError(t, validateAnalyticsGroupBy([]string{"external_customer_id"}))
	require.NoError(t, validateAnalyticsGroupBy([]string{"meter_id", "external_customer_id", "source", "properties.region"}))
}

func TestValidateAnalyticsGroupBy_PreExistingEntriesStillAccepted(t *testing.T) {
	require.NoError(t, validateAnalyticsGroupBy([]string{"meter_id"}))
	require.NoError(t, validateAnalyticsGroupBy([]string{"source"}))
	require.NoError(t, validateAnalyticsGroupBy([]string{"properties.region"}))
}

func TestValidateAnalyticsGroupBy_RejectsUnknownEntry(t *testing.T) {
	err := validateAnalyticsGroupBy([]string{"customer_id"}) // alias, not the real column name
	require.Error(t, err)
	assert.True(t, ierr.IsValidation(err))
}
