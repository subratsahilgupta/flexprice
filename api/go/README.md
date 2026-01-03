# FlexPrice Go SDK

This is the Go client library for the FlexPrice API.

## Installation

```bash
go get github.com/flexprice/go-sdk
```

## Usage

### Basic API Usage

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	flexprice "github.com/flexprice/go-sdk"
	"github.com/joho/godotenv"
	"github.com/samber/lo"
)

// This sample application demonstrates how to use the FlexPrice Go SDK
// to create and retrieve events, showing the basic patterns for API interaction.
// To run this example:
// 1. Copy this file to your project
// 2. Create a .env file with FLEXPRICE_API_KEY and FLEXPRICE_API_HOST
// 3. Run with: go run main.go

// Sample .env file:
// FLEXPRICE_API_KEY=your_api_key_here
// FLEXPRICE_API_HOST=api.cloud.flexprice.io/v1

func RunSample() {
	// Load .env file if present
	godotenv.Load()

	// Get API credentials from environment
	apiKey := os.Getenv("FLEXPRICE_API_KEY")
	apiHost := os.Getenv("FLEXPRICE_API_HOST")

	if apiKey == "" || apiHost == "" {
		log.Fatal("Missing required environment variables: FLEXPRICE_API_KEY and FLEXPRICE_API_HOST")
	}

	// Initialize API client
	config := flexprice.NewConfiguration()
	config.Scheme = "https"
	config.Host = apiHost
	config.AddDefaultHeader("x-api-key", apiKey)

	client := flexprice.NewAPIClient(config)
	ctx := context.Background()

	// Generate a unique customer ID for this sample
	customerId := fmt.Sprintf("sample-customer-%d", time.Now().Unix())

	// Step 1: Create an event
	fmt.Println("Creating event...")
	eventRequest := flexprice.DtoIngestEventRequest{
		EventName:          "Sample Event",
		ExternalCustomerId: customerId,
		Properties: &map[string]string{
			"source":      "sample_app",
			"environment": "test",
			"timestamp":   time.Now().String(),
		},
		Source:    lo.ToPtr("sample_app"),
		Timestamp: lo.ToPtr(time.Now().Format(time.RFC3339)),
	}

	// Send the event creation request
	result, response, err := client.EventsAPI.EventsPost(ctx).
		Event(eventRequest).
		Execute()

	if err != nil {
		log.Fatalf("Error creating event: %v", err)
	}

	if response.StatusCode != 202 {
		log.Fatalf("Expected status code 202, got %d", response.StatusCode)
	}

	// The result is a map, so we need to use map access
	eventId := result["event_id"]
	fmt.Printf("Event created successfully! ID: %v\n\n", eventId)

	// Step 2: Retrieve events for this customer
	fmt.Println("Retrieving events for customer...")
	events, response, err := client.EventsAPI.EventsGet(ctx).
		ExternalCustomerId(customerId).
		Execute()

	if err != nil {
		log.Fatalf("Error retrieving events: %v", err)
	}

	if response.StatusCode != 200 {
		log.Fatalf("Expected status code 200, got %d", response.StatusCode)
	}

	// Process the events (the response is a map)
	fmt.Printf("Raw response: %+v\n\n", response)

	for i, event := range events.Events {
		fmt.Printf("Event %d: %v - %v\n", i+1, event.Id, event.EventName)
		fmt.Printf("Event properties: %v\n", event.Properties)
	}

	fmt.Println("Sample application completed successfully!")
}

func main() {
	RunSample()
}
```

### Async Client Usage

The FlexPrice Go SDK includes an asynchronous client for more efficient event tracking, especially for high-volume applications:

```go
func RunAsyncSample(client *flexprice.APIClient) {
	// Create an AsyncClient with debug enabled
	asyncConfig := flexprice.DefaultAsyncConfig()
	asyncConfig.Debug = true
	asyncClient := client.NewAsyncClientWithConfig(asyncConfig)

	// Example 1: Simple event
	err := asyncClient.Enqueue(
		"api_request",
		"customer-123",
		map[string]interface{}{
			"path":             "/api/resource",
			"method":           "GET",
			"status":           "200",
			"response_time_ms": 150,
		},
	)
	if err != nil {
		log.Fatalf("Failed to enqueue event: %v", err)
	}
	fmt.Println("Enqueued simple event")

	// Example 2: Event with additional options
	err = asyncClient.EnqueueWithOptions(flexprice.EventOptions{
		EventName:          "file_upload",
		ExternalCustomerID: "customer-123",
		CustomerID:         "cust_456",  // Optional internal FlexPrice ID
		EventID:            "event_789", // Custom event ID
		Properties: map[string]interface{}{
			"file_size_bytes": 1048576,
			"file_type":       "image/jpeg",
			"storage_bucket":  "user_uploads",
		},
		Source:    "upload_service",
		Timestamp: time.Now().Format(time.RFC3339),
	})
	if err != nil {
		log.Fatalf("Failed to enqueue event: %v", err)
	}
	fmt.Println("Enqueued event with custom options")

	// Example 3: Batch multiple events
	for i := 0; i < 10; i++ {
		err = asyncClient.Enqueue(
			"batch_example",
			fmt.Sprintf("customer-%d", i),
			map[string]interface{}{
				"index": i,
				"batch": "demo",
			},
		)
		if err != nil {
			log.Fatalf("Failed to enqueue batch event: %v", err)
		}
	}
	fmt.Println("Enqueued 10 batch events")

	// Wait for a moment to let the API requests complete
	fmt.Println("Waiting for events to be processed...")
	time.Sleep(time.Second * 5)
	
	// Explicitly close the client - this will flush any remaining events
	fmt.Println("Closing client...")
	asyncClient.Close()
	
	fmt.Println("Example completed successfully!")
}
```

## Async Client Benefits

The async client provides several advantages:

1. **Efficient Batching**: Events are automatically batched for more efficient API usage
2. **Background Processing**: Events are sent asynchronously, not blocking your application
3. **Auto-Generated IDs**: EventIDs are automatically generated if not provided
4. **Rich Property Types**: Properties support various types (numbers, booleans, etc.)
5. **Detailed Logging**: Enable debug mode for comprehensive logging

## Features

- Complete API coverage
- Type-safe client
- Detailed documentation
- Error handling
- Batch processing for high-volume applications

## Documentation

For detailed API documentation, refer to the code comments and the official FlexPrice API documentation. 
<!-- Start Summary [summary] -->
## Summary

FlexPrice API: FlexPrice API Service
<!-- End Summary [summary] -->

<!-- Start Table of Contents [toc] -->
## Table of Contents
<!-- $toc-max-depth=2 -->
* [FlexPrice Go SDK](#flexprice-go-sdk)
  * [Installation](#installation)
  * [Usage](#usage)
  * [Async Client Benefits](#async-client-benefits)
  * [Features](#features)
  * [Documentation](#documentation)
  * [SDK Installation](#sdk-installation)
  * [SDK Example Usage](#sdk-example-usage)
  * [Authentication](#authentication)
  * [Available Resources and Operations](#available-resources-and-operations)
  * [Retries](#retries)
  * [Error Handling](#error-handling)
  * [Custom HTTP Client](#custom-http-client)

<!-- End Table of Contents [toc] -->

<!-- Start SDK Installation [installation] -->
## SDK Installation

To add the SDK as a dependency to your project:
```bash
go get github.com/flexprice/go-sdk-temp/v2
```
<!-- End SDK Installation [installation] -->

<!-- Start SDK Example Usage [usage] -->
## SDK Example Usage

### Example

```go
package main

import (
	"context"
	"github.com/flexprice/go-sdk-temp/v2"
	"github.com/flexprice/go-sdk-temp/v2/models/operations"
	"log"
)

func main() {
	ctx := context.Background()

	s := v2.New(
		"https://api.example.com",
		v2.WithSecurity("<YOUR_API_KEY_HERE>"),
	)

	res, err := s.Addons.GetAddons(ctx, operations.GetAddonsRequest{})
	if err != nil {
		log.Fatal(err)
	}
	if res != nil {
		// handle response
	}
}

```
<!-- End SDK Example Usage [usage] -->

<!-- Start Authentication [security] -->
## Authentication

### Per-Client Security Schemes

This SDK supports the following security scheme globally:

| Name         | Type   | Scheme  |
| ------------ | ------ | ------- |
| `APIKeyAuth` | apiKey | API key |

You can configure it using the `WithSecurity` option when initializing the SDK client instance. For example:
```go
package main

import (
	"context"
	"github.com/flexprice/go-sdk-temp/v2"
	"github.com/flexprice/go-sdk-temp/v2/models/operations"
	"log"
)

func main() {
	ctx := context.Background()

	s := v2.New(
		"https://api.example.com",
		v2.WithSecurity("<YOUR_API_KEY_HERE>"),
	)

	res, err := s.Addons.GetAddons(ctx, operations.GetAddonsRequest{})
	if err != nil {
		log.Fatal(err)
	}
	if res != nil {
		// handle response
	}
}

```
<!-- End Authentication [security] -->

<!-- Start Available Resources and Operations [operations] -->
## Available Resources and Operations

<details open>
<summary>Available methods</summary>

### [Addons](docs/sdks/addons/README.md)

* [GetAddons](docs/sdks/addons/README.md#getaddons) - List addons
* [PostAddons](docs/sdks/addons/README.md#postaddons) - Create addon
* [GetAddonsLookupLookupKey](docs/sdks/addons/README.md#getaddonslookuplookupkey) - Get addon by lookup key
* [PostAddonsSearch](docs/sdks/addons/README.md#postaddonssearch) - List addons by filter
* [GetAddonsID](docs/sdks/addons/README.md#getaddonsid) - Get addon
* [PutAddonsID](docs/sdks/addons/README.md#putaddonsid) - Update addon
* [DeleteAddonsID](docs/sdks/addons/README.md#deleteaddonsid) - Delete addon

### [AlertLogs](docs/sdks/alertlogs/README.md)

* [PostAlertSearch](docs/sdks/alertlogs/README.md#postalertsearch) - List alert logs by filter

### [Auth](docs/sdks/auth/README.md)

* [PostAuthLogin](docs/sdks/auth/README.md#postauthlogin) - Login
* [PostAuthSignup](docs/sdks/auth/README.md#postauthsignup) - Sign up

### [Connections](docs/sdks/connections/README.md)

* [GetConnections](docs/sdks/connections/README.md#getconnections) - Get connections
* [PostConnectionsSearch](docs/sdks/connections/README.md#postconnectionssearch) - List connections by filter
* [GetConnectionsID](docs/sdks/connections/README.md#getconnectionsid) - Get a connection
* [PutConnectionsID](docs/sdks/connections/README.md#putconnectionsid) - Update a connection
* [DeleteConnectionsID](docs/sdks/connections/README.md#deleteconnectionsid) - Delete a connection

### [Costs](docs/sdks/costs/README.md)

* [PostCosts](docs/sdks/costs/README.md#postcosts) - Create a new costsheet
* [GetCostsActive](docs/sdks/costs/README.md#getcostsactive) - Get active costsheet for tenant
* [PostCostsAnalytics](docs/sdks/costs/README.md#postcostsanalytics) - Get combined revenue and cost analytics
* [PostCostsAnalyticsV2](docs/sdks/costs/README.md#postcostsanalyticsv2) - Get combined revenue and cost analytics
* [PostCostsSearch](docs/sdks/costs/README.md#postcostssearch) - List costsheets by filter
* [GetCostsID](docs/sdks/costs/README.md#getcostsid) - Get a costsheet by ID
* [PutCostsID](docs/sdks/costs/README.md#putcostsid) - Update a costsheet
* [DeleteCostsID](docs/sdks/costs/README.md#deletecostsid) - Delete a costsheet

### [Coupons](docs/sdks/coupons/README.md)

* [GetCoupons](docs/sdks/coupons/README.md#getcoupons) - List coupons with filtering
* [PostCoupons](docs/sdks/coupons/README.md#postcoupons) - Create a new coupon
* [GetCouponsID](docs/sdks/coupons/README.md#getcouponsid) - Get a coupon by ID
* [PutCouponsID](docs/sdks/coupons/README.md#putcouponsid) - Update a coupon
* [DeleteCouponsID](docs/sdks/coupons/README.md#deletecouponsid) - Delete a coupon

### [CreditNotes](docs/sdks/creditnotes/README.md)

* [GetCreditnotes](docs/sdks/creditnotes/README.md#getcreditnotes) - List credit notes with filtering
* [PostCreditnotes](docs/sdks/creditnotes/README.md#postcreditnotes) - Create a new credit note
* [GetCreditnotesID](docs/sdks/creditnotes/README.md#getcreditnotesid) - Get a credit note by ID
* [PostCreditnotesIDFinalize](docs/sdks/creditnotes/README.md#postcreditnotesidfinalize) - Process a draft credit note
* [PostCreditnotesIDVoid](docs/sdks/creditnotes/README.md#postcreditnotesidvoid) - Void a credit note

### [CreditGrants](docs/sdks/creditgrants/README.md)

* [GetCreditgrants](docs/sdks/creditgrants/README.md#getcreditgrants) - Get credit grants
* [PostCreditgrants](docs/sdks/creditgrants/README.md#postcreditgrants) - Create a new credit grant
* [GetCreditgrantsID](docs/sdks/creditgrants/README.md#getcreditgrantsid) - Get a credit grant by ID
* [PutCreditgrantsID](docs/sdks/creditgrants/README.md#putcreditgrantsid) - Update a credit grant
* [DeleteCreditgrantsID](docs/sdks/creditgrants/README.md#deletecreditgrantsid) - Delete a credit grant
* [GetPlansIDCreditgrants](docs/sdks/creditgrants/README.md#getplansidcreditgrants) - Get plan credit grants

### [Customers](docs/sdks/customers/README.md)

* [GetCustomers](docs/sdks/customers/README.md#getcustomers) - Get customers
* [PostCustomers](docs/sdks/customers/README.md#postcustomers) - Create a customer
* [GetCustomersExternalExternalID](docs/sdks/customers/README.md#getcustomersexternalexternalid) - Get a customer by external id
* [PostCustomersSearch](docs/sdks/customers/README.md#postcustomerssearch) - List customers by filter
* [GetCustomersUsage](docs/sdks/customers/README.md#getcustomersusage) - Get customer usage summary
* [GetCustomersID](docs/sdks/customers/README.md#getcustomersid) - Get a customer
* [PutCustomersID](docs/sdks/customers/README.md#putcustomersid) - Update a customer
* [DeleteCustomersID](docs/sdks/customers/README.md#deletecustomersid) - Delete a customer
* [GetCustomersIDEntitlements](docs/sdks/customers/README.md#getcustomersidentitlements) - Get customer entitlements
* [GetCustomersIDGrantsUpcoming](docs/sdks/customers/README.md#getcustomersidgrantsupcoming) - Get upcoming credit grant applications

### [Entitlements](docs/sdks/entitlements/README.md)

* [GetAddonsIDEntitlements](docs/sdks/entitlements/README.md#getaddonsidentitlements) - Get addon entitlements
* [GetEntitlements](docs/sdks/entitlements/README.md#getentitlements) - Get entitlements
* [PostEntitlements](docs/sdks/entitlements/README.md#postentitlements) - Create a new entitlement
* [PostEntitlementsBulk](docs/sdks/entitlements/README.md#postentitlementsbulk) - Create multiple entitlements in bulk
* [PostEntitlementsSearch](docs/sdks/entitlements/README.md#postentitlementssearch) - List entitlements by filter
* [GetEntitlementsID](docs/sdks/entitlements/README.md#getentitlementsid) - Get an entitlement by ID
* [PutEntitlementsID](docs/sdks/entitlements/README.md#putentitlementsid) - Update an entitlement
* [DeleteEntitlementsID](docs/sdks/entitlements/README.md#deleteentitlementsid) - Delete an entitlement
* [GetPlansIDEntitlements](docs/sdks/entitlements/README.md#getplansidentitlements) - Get plan entitlements

### [EntityIntegrationMappings](docs/sdks/entityintegrationmappings/README.md)

* [GetEntityIntegrationMappings](docs/sdks/entityintegrationmappings/README.md#getentityintegrationmappings) - List entity integration mappings
* [PostEntityIntegrationMappings](docs/sdks/entityintegrationmappings/README.md#postentityintegrationmappings) - Create entity integration mapping
* [GetEntityIntegrationMappingsID](docs/sdks/entityintegrationmappings/README.md#getentityintegrationmappingsid) - Get entity integration mapping
* [DeleteEntityIntegrationMappingsID](docs/sdks/entityintegrationmappings/README.md#deleteentityintegrationmappingsid) - Delete entity integration mapping

### [Environments](docs/sdks/environments/README.md)

* [GetEnvironments](docs/sdks/environments/README.md#getenvironments) - Get environments
* [PostEnvironments](docs/sdks/environments/README.md#postenvironments) - Create an environment
* [GetEnvironmentsID](docs/sdks/environments/README.md#getenvironmentsid) - Get an environment
* [PutEnvironmentsID](docs/sdks/environments/README.md#putenvironmentsid) - Update an environment

### [Events](docs/sdks/events/README.md)

* [PostEvents](docs/sdks/events/README.md#postevents) - Ingest event
* [PostEventsAnalytics](docs/sdks/events/README.md#posteventsanalytics) - Get usage analytics
* [PostEventsBulk](docs/sdks/events/README.md#posteventsbulk) - Bulk Ingest events
* [PostEventsHuggingfaceInference](docs/sdks/events/README.md#posteventshuggingfaceinference) - Get hugging face inference data
* [GetEventsMonitoring](docs/sdks/events/README.md#geteventsmonitoring) - Get monitoring data
* [PostEventsQuery](docs/sdks/events/README.md#posteventsquery) - List raw events
* [PostEventsUsage](docs/sdks/events/README.md#posteventsusage) - Get usage statistics
* [PostEventsUsageMeter](docs/sdks/events/README.md#posteventsusagemeter) - Get usage by meter

### [Features](docs/sdks/features/README.md)

* [GetFeatures](docs/sdks/features/README.md#getfeatures) - List features
* [PostFeatures](docs/sdks/features/README.md#postfeatures) - Create a new feature
* [PostFeaturesSearch](docs/sdks/features/README.md#postfeaturessearch) - List features by filter
* [GetFeaturesID](docs/sdks/features/README.md#getfeaturesid) - Get a feature by ID
* [PutFeaturesID](docs/sdks/features/README.md#putfeaturesid) - Update a feature
* [DeleteFeaturesID](docs/sdks/features/README.md#deletefeaturesid) - Delete a feature

### [Groups](docs/sdks/groups/README.md)

* [PostGroups](docs/sdks/groups/README.md#postgroups) - Create a group
* [PostGroupsSearch](docs/sdks/groups/README.md#postgroupssearch) - Get groups
* [GetGroupsID](docs/sdks/groups/README.md#getgroupsid) - Get a group
* [DeleteGroupsID](docs/sdks/groups/README.md#deletegroupsid) - Delete a group

### [Integrations](docs/sdks/integrations/README.md)

* [GetSecretsIntegrationsByProviderProvider](docs/sdks/integrations/README.md#getsecretsintegrationsbyproviderprovider) - Get integration details
* [PostSecretsIntegrationsCreateProvider](docs/sdks/integrations/README.md#postsecretsintegrationscreateprovider) - Create or update an integration
* [GetSecretsIntegrationsLinked](docs/sdks/integrations/README.md#getsecretsintegrationslinked) - List linked integrations
* [DeleteSecretsIntegrationsID](docs/sdks/integrations/README.md#deletesecretsintegrationsid) - Delete an integration

### [Invoices](docs/sdks/invoices/README.md)

* [GetCustomersIDInvoicesSummary](docs/sdks/invoices/README.md#getcustomersidinvoicessummary) - Get a customer invoice summary
* [GetInvoices](docs/sdks/invoices/README.md#getinvoices) - List invoices
* [PostInvoices](docs/sdks/invoices/README.md#postinvoices) - Create a new one off invoice
* [PostInvoicesPreview](docs/sdks/invoices/README.md#postinvoicespreview) - Get a preview invoice
* [PostInvoicesSearch](docs/sdks/invoices/README.md#postinvoicessearch) - List invoices by filter
* [GetInvoicesID](docs/sdks/invoices/README.md#getinvoicesid) - Get an invoice by ID
* [PutInvoicesID](docs/sdks/invoices/README.md#putinvoicesid) - Update an invoice
* [PostInvoicesIDCommsTrigger](docs/sdks/invoices/README.md#postinvoicesidcommstrigger) - Trigger communication webhook for an invoice
* [PostInvoicesIDFinalize](docs/sdks/invoices/README.md#postinvoicesidfinalize) - Finalize an invoice
* [PutInvoicesIDPayment](docs/sdks/invoices/README.md#putinvoicesidpayment) - Update invoice payment status
* [PostInvoicesIDPaymentAttempt](docs/sdks/invoices/README.md#postinvoicesidpaymentattempt) - Attempt payment for an invoice
* [GetInvoicesIDPdf](docs/sdks/invoices/README.md#getinvoicesidpdf) - Get PDF for an invoice
* [PostInvoicesIDRecalculate](docs/sdks/invoices/README.md#postinvoicesidrecalculate) - Recalculate invoice totals and line items
* [PostInvoicesIDVoid](docs/sdks/invoices/README.md#postinvoicesidvoid) - Void an invoice

### [Payments](docs/sdks/payments/README.md)

* [GetPayments](docs/sdks/payments/README.md#getpayments) - List payments
* [PostPayments](docs/sdks/payments/README.md#postpayments) - Create a new payment
* [GetPaymentsID](docs/sdks/payments/README.md#getpaymentsid) - Get a payment by ID
* [PutPaymentsID](docs/sdks/payments/README.md#putpaymentsid) - Update a payment
* [DeletePaymentsID](docs/sdks/payments/README.md#deletepaymentsid) - Delete a payment
* [PostPaymentsIDProcess](docs/sdks/payments/README.md#postpaymentsidprocess) - Process a payment

### [Plans](docs/sdks/plans/README.md)

* [GetPlans](docs/sdks/plans/README.md#getplans) - Get plans
* [PostPlans](docs/sdks/plans/README.md#postplans) - Create a new plan
* [PostPlansSearch](docs/sdks/plans/README.md#postplanssearch) - List plans by filter
* [GetPlansID](docs/sdks/plans/README.md#getplansid) - Get a plan
* [PutPlansID](docs/sdks/plans/README.md#putplansid) - Update a plan
* [DeletePlansID](docs/sdks/plans/README.md#deleteplansid) - Delete a plan
* [PostPlansIDSyncSubscriptions](docs/sdks/plans/README.md#postplansidsyncsubscriptions) - Synchronize plan prices

### [PriceUnits](docs/sdks/priceunits/README.md)

* [GetPricesUnits](docs/sdks/priceunits/README.md#getpricesunits) - List price units
* [PostPricesUnits](docs/sdks/priceunits/README.md#postpricesunits) - Create a new price unit
* [GetPricesUnitsCodeCode](docs/sdks/priceunits/README.md#getpricesunitscodecode) - Get a price unit by code
* [PostPricesUnitsSearch](docs/sdks/priceunits/README.md#postpricesunitssearch) - List price units by filter
* [GetPricesUnitsID](docs/sdks/priceunits/README.md#getpricesunitsid) - Get a price unit by ID
* [PutPricesUnitsID](docs/sdks/priceunits/README.md#putpricesunitsid) - Update a price unit
* [DeletePricesUnitsID](docs/sdks/priceunits/README.md#deletepricesunitsid) - Delete a price unit

### [Prices](docs/sdks/prices/README.md)

* [GetPrices](docs/sdks/prices/README.md#getprices) - Get prices
* [PostPrices](docs/sdks/prices/README.md#postprices) - Create a new price
* [PostPricesBulk](docs/sdks/prices/README.md#postpricesbulk) - Create multiple prices in bulk
* [GetPricesID](docs/sdks/prices/README.md#getpricesid) - Get a price by ID
* [PutPricesID](docs/sdks/prices/README.md#putpricesid) - Update a price
* [DeletePricesID](docs/sdks/prices/README.md#deletepricesid) - Delete a price

### [Rbac](docs/sdks/rbac/README.md)

* [GetRbacRoles](docs/sdks/rbac/README.md#getrbacroles) - List all RBAC roles
* [GetRbacRolesID](docs/sdks/rbac/README.md#getrbacrolesid) - Get a specific RBAC role

### [ScheduledTasks](docs/sdks/scheduledtasks/README.md)

* [GetTasksScheduled](docs/sdks/scheduledtasks/README.md#gettasksscheduled) - List scheduled tasks
* [PostTasksScheduled](docs/sdks/scheduledtasks/README.md#posttasksscheduled) - Create a scheduled task
* [PostTasksScheduledScheduleUpdateBillingPeriod](docs/sdks/scheduledtasks/README.md#posttasksscheduledscheduleupdatebillingperiod) - Schedule update billing period
* [GetTasksScheduledID](docs/sdks/scheduledtasks/README.md#gettasksscheduledid) - Get a scheduled task
* [PutTasksScheduledID](docs/sdks/scheduledtasks/README.md#puttasksscheduledid) - Update a scheduled task
* [DeleteTasksScheduledID](docs/sdks/scheduledtasks/README.md#deletetasksscheduledid) - Delete a scheduled task
* [PostTasksScheduledIDRun](docs/sdks/scheduledtasks/README.md#posttasksscheduledidrun) - Trigger force run

### [Secrets](docs/sdks/secrets/README.md)

* [GetSecretsAPIKeys](docs/sdks/secrets/README.md#getsecretsapikeys) - List API keys
* [PostSecretsAPIKeys](docs/sdks/secrets/README.md#postsecretsapikeys) - Create a new API key
* [DeleteSecretsAPIKeysID](docs/sdks/secrets/README.md#deletesecretsapikeysid) - Delete an API key

### [Subscriptions](docs/sdks/subscriptions/README.md)

* [GetSubscriptions](docs/sdks/subscriptions/README.md#getsubscriptions) - List subscriptions
* [PostSubscriptions](docs/sdks/subscriptions/README.md#postsubscriptions) - Create subscription
* [PostSubscriptionsAddon](docs/sdks/subscriptions/README.md#postsubscriptionsaddon) - Add addon to subscription
* [DeleteSubscriptionsAddon](docs/sdks/subscriptions/README.md#deletesubscriptionsaddon) - Remove addon from subscription
* [PutSubscriptionsLineitemsID](docs/sdks/subscriptions/README.md#putsubscriptionslineitemsid) - Update subscription line item
* [DeleteSubscriptionsLineitemsID](docs/sdks/subscriptions/README.md#deletesubscriptionslineitemsid) - Delete subscription line item
* [PostSubscriptionsSearch](docs/sdks/subscriptions/README.md#postsubscriptionssearch) - List subscriptions by filter
* [PostSubscriptionsUsage](docs/sdks/subscriptions/README.md#postsubscriptionsusage) - Get usage by subscription
* [GetSubscriptionsID](docs/sdks/subscriptions/README.md#getsubscriptionsid) - Get subscription
* [PostSubscriptionsIDActivate](docs/sdks/subscriptions/README.md#postsubscriptionsidactivate) - Activate draft subscription
* [GetSubscriptionsIDAddonsAssociations](docs/sdks/subscriptions/README.md#getsubscriptionsidaddonsassociations) - Get active addon associations
* [PostSubscriptionsIDCancel](docs/sdks/subscriptions/README.md#postsubscriptionsidcancel) - Cancel subscription
* [PostSubscriptionsIDChangeExecute](docs/sdks/subscriptions/README.md#postsubscriptionsidchangeexecute) - Execute subscription plan change
* [PostSubscriptionsIDChangePreview](docs/sdks/subscriptions/README.md#postsubscriptionsidchangepreview) - Preview subscription plan change
* [GetSubscriptionsIDEntitlements](docs/sdks/subscriptions/README.md#getsubscriptionsidentitlements) - Get subscription entitlements
* [GetSubscriptionsIDGrantsUpcoming](docs/sdks/subscriptions/README.md#getsubscriptionsidgrantsupcoming) - Get upcoming credit grant applications
* [PostSubscriptionsIDPause](docs/sdks/subscriptions/README.md#postsubscriptionsidpause) - Pause a subscription
* [GetSubscriptionsIDPauses](docs/sdks/subscriptions/README.md#getsubscriptionsidpauses) - List all pauses for a subscription
* [PostSubscriptionsIDResume](docs/sdks/subscriptions/README.md#postsubscriptionsidresume) - Resume a paused subscription

### [Tasks](docs/sdks/tasks/README.md)

* [GetTasks](docs/sdks/tasks/README.md#gettasks) - List tasks
* [PostTasks](docs/sdks/tasks/README.md#posttasks) - Create a new task
* [GetTasksResult](docs/sdks/tasks/README.md#gettasksresult) - Get task processing result
* [GetTasksID](docs/sdks/tasks/README.md#gettasksid) - Get a task
* [PutTasksIDStatus](docs/sdks/tasks/README.md#puttasksidstatus) - Update task status

### [TaxAssociations](docs/sdks/taxassociations/README.md)

* [GetTaxesAssociations](docs/sdks/taxassociations/README.md#gettaxesassociations) - List tax associations
* [PostTaxesAssociations](docs/sdks/taxassociations/README.md#posttaxesassociations) - Create Tax Association
* [GetTaxesAssociationsID](docs/sdks/taxassociations/README.md#gettaxesassociationsid) - Get Tax Association
* [PutTaxesAssociationsID](docs/sdks/taxassociations/README.md#puttaxesassociationsid) - Update tax association
* [DeleteTaxesAssociationsID](docs/sdks/taxassociations/README.md#deletetaxesassociationsid) - Delete tax association

### [TaxRates](docs/sdks/taxrates/README.md)

* [GetTaxesRates](docs/sdks/taxrates/README.md#gettaxesrates) - Get tax rates
* [PostTaxesRates](docs/sdks/taxrates/README.md#posttaxesrates) - Create a tax rate
* [GetTaxesRatesID](docs/sdks/taxrates/README.md#gettaxesratesid) - Get a tax rate
* [PutTaxesRatesID](docs/sdks/taxrates/README.md#puttaxesratesid) - Update a tax rate
* [DeleteTaxesRatesID](docs/sdks/taxrates/README.md#deletetaxesratesid) - Delete a tax rate

### [Tenants](docs/sdks/tenants/README.md)

* [GetTenantBilling](docs/sdks/tenants/README.md#gettenantbilling) - Get billing usage for the current tenant
* [PostTenants](docs/sdks/tenants/README.md#posttenants) - Create a new tenant
* [PutTenantsUpdate](docs/sdks/tenants/README.md#puttenantsupdate) - Update a tenant
* [GetTenantsID](docs/sdks/tenants/README.md#gettenantsid) - Get tenant by ID

### [Users](docs/sdks/users/README.md)

* [PostUsers](docs/sdks/users/README.md#postusers) - Create service account
* [GetUsersMe](docs/sdks/users/README.md#getusersme) - Get user info
* [PostUsersSearch](docs/sdks/users/README.md#postuserssearch) - List users with filters

### [Wallets](docs/sdks/wallets/README.md)

* [GetCustomersWallets](docs/sdks/wallets/README.md#getcustomerswallets) - Get Customer Wallets
* [GetCustomersIDWallets](docs/sdks/wallets/README.md#getcustomersidwallets) - Get wallets by customer ID
* [GetWallets](docs/sdks/wallets/README.md#getwallets) - List wallets
* [PostWallets](docs/sdks/wallets/README.md#postwallets) - Create a new wallet
* [PostWalletsSearch](docs/sdks/wallets/README.md#postwalletssearch) - List wallets by filter
* [PostWalletsTransactionsSearch](docs/sdks/wallets/README.md#postwalletstransactionssearch) - List wallet transactions by filter
* [GetWalletsID](docs/sdks/wallets/README.md#getwalletsid) - Get wallet by ID
* [PutWalletsID](docs/sdks/wallets/README.md#putwalletsid) - Update a wallet
* [GetWalletsIDBalanceRealTime](docs/sdks/wallets/README.md#getwalletsidbalancerealtime) - Get wallet balance
* [PostWalletsIDTerminate](docs/sdks/wallets/README.md#postwalletsidterminate) - Terminate a wallet
* [PostWalletsIDTopUp](docs/sdks/wallets/README.md#postwalletsidtopup) - Top up wallet
* [GetWalletsIDTransactions](docs/sdks/wallets/README.md#getwalletsidtransactions) - Get wallet transactions

### [Webhooks](docs/sdks/webhooks/README.md)

* [PostWebhooksChargebeeTenantIDEnvironmentID](docs/sdks/webhooks/README.md#postwebhookschargebeetenantidenvironmentid) - Handle Chargebee webhook events
* [PostWebhooksHubspotTenantIDEnvironmentID](docs/sdks/webhooks/README.md#postwebhookshubspottenantidenvironmentid) - Handle HubSpot webhook events
* [PostWebhooksNomodTenantIDEnvironmentID](docs/sdks/webhooks/README.md#postwebhooksnomodtenantidenvironmentid) - Handle Nomod webhook events
* [PostWebhooksQuickbooksTenantIDEnvironmentID](docs/sdks/webhooks/README.md#postwebhooksquickbookstenantidenvironmentid) - Handle QuickBooks webhook events
* [PostWebhooksRazorpayTenantIDEnvironmentID](docs/sdks/webhooks/README.md#postwebhooksrazorpaytenantidenvironmentid) - Handle Razorpay webhook events
* [PostWebhooksStripeTenantIDEnvironmentID](docs/sdks/webhooks/README.md#postwebhooksstripetenantidenvironmentid) - Handle Stripe webhook events

</details>
<!-- End Available Resources and Operations [operations] -->

<!-- Start Retries [retries] -->
## Retries

Some of the endpoints in this SDK support retries. If you use the SDK without any configuration, it will fall back to the default retry strategy provided by the API. However, the default retry strategy can be overridden on a per-operation basis, or across the entire SDK.

To change the default retry strategy for a single API call, simply provide a `retry.Config` object to the call by using the `WithRetries` option:
```go
package main

import (
	"context"
	"github.com/flexprice/go-sdk-temp/v2"
	"github.com/flexprice/go-sdk-temp/v2/models/operations"
	"github.com/flexprice/go-sdk-temp/v2/retry"
	"log"
	"models/operations"
)

func main() {
	ctx := context.Background()

	s := v2.New(
		"https://api.example.com",
		v2.WithSecurity("<YOUR_API_KEY_HERE>"),
	)

	res, err := s.Addons.GetAddons(ctx, operations.GetAddonsRequest{}, operations.WithRetries(
		retry.Config{
			Strategy: "backoff",
			Backoff: &retry.BackoffStrategy{
				InitialInterval: 1,
				MaxInterval:     50,
				Exponent:        1.1,
				MaxElapsedTime:  100,
			},
			RetryConnectionErrors: false,
		}))
	if err != nil {
		log.Fatal(err)
	}
	if res != nil {
		// handle response
	}
}

```

If you'd like to override the default retry strategy for all operations that support retries, you can use the `WithRetryConfig` option at SDK initialization:
```go
package main

import (
	"context"
	"github.com/flexprice/go-sdk-temp/v2"
	"github.com/flexprice/go-sdk-temp/v2/models/operations"
	"github.com/flexprice/go-sdk-temp/v2/retry"
	"log"
)

func main() {
	ctx := context.Background()

	s := v2.New(
		"https://api.example.com",
		v2.WithRetryConfig(
			retry.Config{
				Strategy: "backoff",
				Backoff: &retry.BackoffStrategy{
					InitialInterval: 1,
					MaxInterval:     50,
					Exponent:        1.1,
					MaxElapsedTime:  100,
				},
				RetryConnectionErrors: false,
			}),
		v2.WithSecurity("<YOUR_API_KEY_HERE>"),
	)

	res, err := s.Addons.GetAddons(ctx, operations.GetAddonsRequest{})
	if err != nil {
		log.Fatal(err)
	}
	if res != nil {
		// handle response
	}
}

```
<!-- End Retries [retries] -->

<!-- Start Error Handling [errors] -->
## Error Handling

Handling errors in this SDK should largely match your expectations. All operations return a response object or an error, they will never return both.

By Default, an API error will return `sdkerrors.APIError`. When custom error responses are specified for an operation, the SDK may also return their associated error. You can refer to respective *Errors* tables in SDK docs for more details on possible error types for each operation.

For example, the `GetAddons` function may return the following errors:

| Error Type                    | Status Code | Content Type     |
| ----------------------------- | ----------- | ---------------- |
| sdkerrors.ErrorsErrorResponse | 400         | application/json |
| sdkerrors.ErrorsErrorResponse | 500         | application/json |
| sdkerrors.APIError            | 4XX, 5XX    | \*/\*            |

### Example

```go
package main

import (
	"context"
	"errors"
	"github.com/flexprice/go-sdk-temp/v2"
	"github.com/flexprice/go-sdk-temp/v2/models/operations"
	"github.com/flexprice/go-sdk-temp/v2/models/sdkerrors"
	"log"
)

func main() {
	ctx := context.Background()

	s := v2.New(
		"https://api.example.com",
		v2.WithSecurity("<YOUR_API_KEY_HERE>"),
	)

	res, err := s.Addons.GetAddons(ctx, operations.GetAddonsRequest{})
	if err != nil {

		var e *sdkerrors.ErrorsErrorResponse
		if errors.As(err, &e) {
			// handle error
			log.Fatal(e.Error())
		}

		var e *sdkerrors.ErrorsErrorResponse
		if errors.As(err, &e) {
			// handle error
			log.Fatal(e.Error())
		}

		var e *sdkerrors.APIError
		if errors.As(err, &e) {
			// handle error
			log.Fatal(e.Error())
		}
	}
}

```
<!-- End Error Handling [errors] -->

<!-- Start Custom HTTP Client [http-client] -->
## Custom HTTP Client

The Go SDK makes API calls that wrap an internal HTTP client. The requirements for the HTTP client are very simple. It must match this interface:

```go
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}
```

The built-in `net/http` client satisfies this interface and a default client based on the built-in is provided by default. To replace this default with a client of your own, you can implement this interface yourself or provide your own client configured as desired. Here's a simple example, which adds a client with a 30 second timeout.

```go
import (
	"net/http"
	"time"

	"github.com/flexprice/go-sdk-temp/v2"
)

var (
	httpClient = &http.Client{Timeout: 30 * time.Second}
	sdkClient  = v2.New(v2.WithClient(httpClient))
)
```

This can be a convenient way to configure timeouts, cookies, proxies, custom headers, and other low-level configuration.
<!-- End Custom HTTP Client [http-client] -->

<!-- Placeholder for Future Speakeasy SDK Sections -->
