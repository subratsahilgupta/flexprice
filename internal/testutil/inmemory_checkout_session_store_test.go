package testutil

import (
	"testing"
	"time"

	domainCheckout "github.com/flexprice/flexprice/internal/domain/checkout"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestInMemoryCheckoutSessionStore_List_ConfigurationFilter(t *testing.T) {
	ctx := types.SetTenantID(t.Context(), "tenant_test")
	ctx = types.SetEnvironmentID(ctx, "env_test")
	store := NewInMemoryCheckoutSessionStore()

	base := types.GetDefaultBaseModel(ctx)
	now := time.Now().UTC()

	olderWallet := &domainCheckout.CheckoutSession{
		ID:              "cs_wallet_old",
		EnvironmentID:   types.GetEnvironmentID(ctx),
		CustomerID:      "cust_1",
		Action:          types.CheckoutActionWalletTopup,
		CheckoutStatus:  types.CheckoutStatusPending,
		PaymentProvider: types.CheckoutPaymentProviderRazorpay,
		Configuration: domainCheckout.ToJSONBCheckoutConfiguration(types.CheckoutConfiguration{
			WalletTopupParams: &types.WalletTopupParams{WalletID: "wallet_a"},
		}),
		ExpiresAt: now.Add(time.Hour),
		BaseModel: base,
	}
	olderWallet.CreatedAt = now.Add(-2 * time.Hour)

	newerWallet := &domainCheckout.CheckoutSession{
		ID:              "cs_wallet_new",
		EnvironmentID:   types.GetEnvironmentID(ctx),
		CustomerID:      "cust_1",
		Action:          types.CheckoutActionWalletTopup,
		CheckoutStatus:  types.CheckoutStatusPending,
		PaymentProvider: types.CheckoutPaymentProviderRazorpay,
		Configuration: domainCheckout.ToJSONBCheckoutConfiguration(types.CheckoutConfiguration{
			WalletTopupParams: &types.WalletTopupParams{WalletID: "wallet_a"},
		}),
		ExpiresAt: now.Add(time.Hour),
		BaseModel: base,
	}
	newerWallet.CreatedAt = now.Add(-time.Hour)

	otherWallet := &domainCheckout.CheckoutSession{
		ID:              "cs_wallet_other",
		EnvironmentID:   types.GetEnvironmentID(ctx),
		CustomerID:      "cust_1",
		Action:          types.CheckoutActionWalletTopup,
		CheckoutStatus:  types.CheckoutStatusPending,
		PaymentProvider: types.CheckoutPaymentProviderRazorpay,
		Configuration: domainCheckout.ToJSONBCheckoutConfiguration(types.CheckoutConfiguration{
			WalletTopupParams: &types.WalletTopupParams{WalletID: "wallet_b"},
		}),
		ExpiresAt: now.Add(time.Hour),
		BaseModel: base,
	}
	otherWallet.CreatedAt = now

	completed := &domainCheckout.CheckoutSession{
		ID:              "cs_wallet_done",
		EnvironmentID:   types.GetEnvironmentID(ctx),
		CustomerID:      "cust_1",
		Action:          types.CheckoutActionWalletTopup,
		CheckoutStatus:  types.CheckoutStatusCompleted,
		PaymentProvider: types.CheckoutPaymentProviderRazorpay,
		Configuration: domainCheckout.ToJSONBCheckoutConfiguration(types.CheckoutConfiguration{
			WalletTopupParams: &types.WalletTopupParams{WalletID: "wallet_a"},
		}),
		ExpiresAt: now.Add(time.Hour),
		BaseModel: base,
	}
	completed.CreatedAt = now.Add(time.Minute)

	archived := &domainCheckout.CheckoutSession{
		ID:              "cs_wallet_archived",
		EnvironmentID:   types.GetEnvironmentID(ctx),
		CustomerID:      "cust_1",
		Action:          types.CheckoutActionWalletTopup,
		CheckoutStatus:  types.CheckoutStatusPending,
		PaymentProvider: types.CheckoutPaymentProviderRazorpay,
		Configuration: domainCheckout.ToJSONBCheckoutConfiguration(types.CheckoutConfiguration{
			WalletTopupParams: &types.WalletTopupParams{WalletID: "wallet_a"},
		}),
		ExpiresAt: now.Add(time.Hour),
		BaseModel: base,
	}
	archived.Status = types.StatusArchived
	archived.CreatedAt = now.Add(2 * time.Minute)

	modifySub := &domainCheckout.CheckoutSession{
		ID:              "cs_mod_sub",
		EnvironmentID:   types.GetEnvironmentID(ctx),
		CustomerID:      "cust_1",
		Action:          types.CheckoutActionModifySubscription,
		CheckoutStatus:  types.CheckoutStatusPending,
		PaymentProvider: types.CheckoutPaymentProviderRazorpay,
		Configuration: domainCheckout.ToJSONBCheckoutConfiguration(types.CheckoutConfiguration{
			ModifySubscriptionParams: &types.ModifySubscriptionParams{SubscriptionID: "sub_1"},
		}),
		ExpiresAt: now.Add(time.Hour),
		BaseModel: base,
	}
	modifySub.CreatedAt = now.Add(3 * time.Minute)

	for _, sess := range []*domainCheckout.CheckoutSession{
		olderWallet, newerWallet, otherWallet, completed, archived, modifySub,
	} {
		require.NoError(t, store.Create(ctx, sess))
	}

	activeStatuses := []types.CheckoutStatus{
		types.CheckoutStatusInitiated,
		types.CheckoutStatusPending,
	}

	listPending := func(cfg *types.CheckoutConfigurationFilter, action types.CheckoutAction) []*domainCheckout.CheckoutSession {
		f := &types.CheckoutSessionFilter{
			QueryFilter:       types.NewNoLimitPublishedQueryFilter(),
			CustomerIDs:       []string{"cust_1"},
			Actions:           []types.CheckoutAction{action},
			CheckoutStatuses:  activeStatuses,
			Configuration:     cfg,
		}
		f.Limit = lo.ToPtr(1)
		items, err := store.List(ctx, f)
		require.NoError(t, err)
		return items
	}

	got := listPending(&types.CheckoutConfigurationFilter{WalletID: "wallet_a"}, types.CheckoutActionWalletTopup)
	require.Len(t, got, 1)
	require.Equal(t, "cs_wallet_new", got[0].ID)

	gotOther := listPending(&types.CheckoutConfigurationFilter{WalletID: "wallet_b"}, types.CheckoutActionWalletTopup)
	require.Len(t, gotOther, 1)
	require.Equal(t, "cs_wallet_other", gotOther[0].ID)

	gotNone := listPending(&types.CheckoutConfigurationFilter{WalletID: "wallet_missing"}, types.CheckoutActionWalletTopup)
	require.Empty(t, gotNone)

	gotSub := listPending(&types.CheckoutConfigurationFilter{SubscriptionID: "sub_1"}, types.CheckoutActionModifySubscription)
	require.Len(t, gotSub, 1)
	require.Equal(t, "cs_mod_sub", gotSub[0].ID)
}
