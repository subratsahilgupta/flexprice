package v1

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Guards the generated spec: the portal session endpoint must stay in the SDK surface.
func TestSwagger_ExposesCustomerPortalSession(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/swagger/swagger-3-0.json")
	require.NoError(t, err)
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string   `json:"operationId"`
			Tags        []string `json:"tags"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(raw, &spec))
	op, ok := spec.Paths["/customers/portal/{external_id}"]["get"]
	require.True(t, ok, "GET /customers/portal/{external_id} missing from swagger-3-0.json")
	assert.Equal(t, "createCustomerPortalSession", op.OperationID)
	assert.Contains(t, op.Tags, "Customers")
}
