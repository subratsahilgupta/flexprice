package main

import (
	"testing"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
)

func TestSeedACLsEnabled_OnlySCRAM(t *testing.T) {
	assert.True(t, seedACLsEnabled(sarama.SASLTypeSCRAMSHA256))
	assert.True(t, seedACLsEnabled(sarama.SASLTypeSCRAMSHA512))
	// OAUTHBEARER (GCP) must NOT seed — even though UseSASL is true there.
	assert.False(t, seedACLsEnabled(sarama.SASLTypeOAuth))
	// Plaintext / PLAIN dev must NOT seed.
	assert.False(t, seedACLsEnabled(sarama.SASLTypePlaintext))
	assert.False(t, seedACLsEnabled(""))
}
