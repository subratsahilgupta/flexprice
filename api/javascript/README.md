# FlexPrice JavaScript/TypeScript SDK

[![npm version](https://badge.fury.io/js/%40flexprice%2Fsdk.svg)](https://badge.fury.io/js/%40flexprice%2Fsdk)
[![TypeScript](https://img.shields.io/badge/TypeScript-Ready-blue.svg)](https://www.typescriptlang.org/)

Official TypeScript/JavaScript SDK for the FlexPrice API with modern ES7 module support and comprehensive type safety.

## Features

- ✅ **Full TypeScript Support** - Complete type definitions for all API endpoints
- ✅ **Modern ES7 Modules** - Native ES modules with CommonJS fallback
- ✅ **Fetch API** - Built on modern web standards
- ✅ **Browser Compatible** - Works in Node.js, Webpack, and Browserify
- ✅ **Promise & Callback Support** - Flexible async patterns
- ✅ **Comprehensive Documentation** - Auto-generated from OpenAPI specs
- ✅ **Error Handling** - Detailed error messages and status codes

## Installation

```bash
npm install @flexprice/sdk --save
```

## Quick Start

```typescript
import { Configuration, EventsApi, CustomersApi } from "@flexprice/sdk";

// Configure the API client
const config = new Configuration({
  basePath: "https://api.cloud.flexprice.io/v1",
  apiKey: "your_api_key_here",
  headers: {
    "X-Environment-ID": "your_environment_id_here",
  },
});

// Create API instances
const eventsApi = new EventsApi(config);
const customersApi = new CustomersApi(config);

// Use APIs directly
const eventRequest = {
  eventName: "user_signup",
  externalCustomerId: "customer-123",
  properties: {
    plan: "premium",
    source: "website",
  },
};

await eventsApi.eventsPost({ event: eventRequest });
```

## Environment Setup

### Environment Variables

Create a `.env` file in your project root:

```bash
# FlexPrice Configuration
FLEXPRICE_API_KEY=sk_your_api_key_here
FLEXPRICE_BASE_URL=https://api.cloud.flexprice.io/v1
FLEXPRICE_ENVIRONMENT_ID=env_your_environment_id_here
```

### Vite/React Applications

For Vite applications, prefix environment variables with `VITE_`:

```bash
# .env
VITE_FLEXPRICE_API_KEY=sk_your_api_key_here
VITE_FLEXPRICE_BASE_URL=https://api.cloud.flexprice.io/v1
VITE_FLEXPRICE_ENVIRONMENT_ID=env_your_environment_id_here
```

```typescript
// config.ts
import { Configuration, EventsApi, CustomersApi } from "@flexprice/sdk";

const API_KEY = import.meta.env.VITE_FLEXPRICE_API_KEY;
const BASE_PATH = import.meta.env.VITE_FLEXPRICE_BASE_URL;
const ENVIRONMENT_ID = import.meta.env.VITE_FLEXPRICE_ENVIRONMENT_ID;

const config = new Configuration({
  basePath: BASE_PATH,
  apiKey: API_KEY,
  headers: {
    "X-Environment-ID": ENVIRONMENT_ID,
  },
});

// Export configured API instances
export const eventsApi = new EventsApi(config);
export const customersApi = new CustomersApi(config);
```

## Available APIs

- **EventsApi** - Event ingestion and analytics
- **CustomersApi** - Customer management
- **AuthApi** - Authentication and user management
- **PlansApi** - Subscription plan management
- **FeaturesApi** - Feature management
- **InvoicesApi** - Invoice operations
- **SubscriptionsApi** - Subscription management
- **AddonsApi** - Addon management
- **CouponsApi** - Coupon management
- **CreditNotesApi** - Credit note management
- **EntitlementsApi** - Feature access control
- **UsersApi** - User management

## API Examples

### Events API

```typescript
import { EventsApi } from "@flexprice/sdk";

const eventsApi = new EventsApi(config);

// Ingest a single event
await eventsApi.eventsPost({
  event: {
    eventName: "api_call",
    externalCustomerId: "customer-123",
    properties: {
      endpoint: "/api/users",
      method: "GET",
      responseTime: 150,
    },
  },
});

// Query events
const events = await eventsApi.eventsQueryPost({
  request: {
    externalCustomerId: "customer-123",
    eventName: "api_call",
    startTime: "2024-01-01T00:00:00Z",
    endTime: "2024-01-31T23:59:59Z",
    limit: 100,
  },
});

// Get usage analytics
const analytics = await eventsApi.eventsAnalyticsPost({
  request: {
    externalCustomerId: "customer-123",
    startTime: "2024-01-01T00:00:00Z",
    endTime: "2024-01-31T23:59:59Z",
    windowSize: "day",
  },
});
```

### Customers API

```typescript
import { CustomersApi } from "@flexprice/sdk";

const customersApi = new CustomersApi(config);

// Create a customer
const customer = await customersApi.customersPost({
  customer: {
    externalId: "customer-123",
    email: "user@example.com",
    name: "John Doe",
    metadata: {
      source: "signup_form",
    },
  },
});

// Get customer
const customerData = await customersApi.customersIdGet({
  id: "customer-123",
});

// Update customer
const updatedCustomer = await customersApi.customersIdPut({
  id: "customer-123",
  customer: {
    name: "John Smith",
    metadata: { plan: "premium" },
  },
});

// List customers
const customers = await customersApi.customersGet({
  limit: 50,
  offset: 0,
  status: "active",
});
```

### Authentication API

```typescript
import { AuthApi } from "@flexprice/sdk";

const authApi = new AuthApi(config);

// Login user
const authResponse = await authApi.authLoginPost({
  login: {
    email: "user@example.com",
    password: "password123",
  },
});

// Sign up new user
const signupResponse = await authApi.authSignupPost({
  signup: {
    email: "newuser@example.com",
    password: "password123",
    name: "New User",
  },
});
```

## React Integration

### With React Query

```typescript
import { useMutation, useQuery } from "@tanstack/react-query";
import { eventsApi } from "./config";

// Fetch events
const { data: events, isLoading } = useQuery({
  queryKey: ["events"],
  queryFn: () =>
    eventsApi.eventsQueryPost({
      request: {
        externalCustomerId: "customer-123",
        limit: 100,
      },
    }),
});

// Fire an event
const { mutate: fireEvent } = useMutation({
  mutationFn: (eventData) => eventsApi.eventsPost({ event: eventData }),
  onSuccess: () => {
    toast.success("Event fired successfully");
  },
  onError: (error) => {
    toast.error("Failed to fire event");
  },
});
```

### With useEffect

```typescript
import { useEffect, useState } from "react";
import { eventsApi } from "./config";

const UsageComponent = () => {
  const [usage, setUsage] = useState(null);

  useEffect(() => {
    const fetchUsage = async () => {
      try {
        const data = await eventsApi.eventsUsagePost({
          request: {
            externalCustomerId: "customer-123",
            startTime: "2024-01-01",
            endTime: "2024-01-31",
          },
        });
        setUsage(data);
      } catch (error) {
        console.error("Failed to fetch usage:", error);
      }
    };

    fetchUsage();
  }, []);

  return <div>{/* Render usage data */}</div>;
};
```

## Error Handling

```typescript
try {
  await eventsApi.eventsPost({ event: eventData });
} catch (error) {
  if (error.status === 401) {
    console.error("Authentication failed");
  } else if (error.status === 400) {
    console.error("Bad request:", error.response?.body);
  } else {
    console.error("API Error:", error.message);
  }
}
```

## TypeScript Support

The SDK includes comprehensive TypeScript definitions:

```typescript
import type {
  DtoIngestEventRequest,
  DtoGetUsageRequest,
  DtoCreateCustomerRequest,
  DtoCustomerResponse,
  // ... many more types
} from "@flexprice/sdk";

// Type-safe event creation
const event: DtoIngestEventRequest = {
  eventName: "llm_usage",
  externalCustomerId: "user_123",
  properties: {
    tokens: 150,
    model: "gpt-4",
  },
};
```

## Browser Usage

```html
<script src="https://cdn.jsdelivr.net/npm/@flexprice/sdk/dist/flexprice-sdk.min.js"></script>
<script>
  // Configure the API client
  const config = new FlexPrice.Configuration({
    basePath: "https://api.cloud.flexprice.io/v1",
    apiKey: "your-api-key-here",
    headers: {
      "X-Environment-ID": "your-environment-id-here",
    },
  });

  // Create API instance
  const eventsApi = new FlexPrice.EventsApi(config);

  // Use the SDK
  eventsApi.eventsPost({
    event: {
      eventName: "page_view",
      externalCustomerId: "user_123",
      properties: { page: "/home" },
    },
  });
</script>
```

## Troubleshooting

### Authentication Issues

- Verify the key is active and has required permissions
- Check that the `x-api-key` header is being sent correctly
- Verify the `X-Environment-ID` header is included

## Support

For support and questions:

- Check the [API Documentation](https://docs.flexprice.io)
- Contact support at [support@flexprice.io](mailto:support@flexprice.io)

<!-- Start Summary [summary] -->
## Summary

FlexPrice API: FlexPrice API Service
<!-- End Summary [summary] -->

<!-- Start Table of Contents [toc] -->
## Table of Contents
<!-- $toc-max-depth=2 -->
* [FlexPrice JavaScript/TypeScript SDK](#flexprice-javascripttypescript-sdk)
  * [Features](#features)
  * [Installation](#installation)
  * [Quick Start](#quick-start)
  * [Environment Setup](#environment-setup)
  * [Available APIs](#available-apis)
  * [API Examples](#api-examples)
  * [React Integration](#react-integration)
  * [Error Handling](#error-handling)
  * [TypeScript Support](#typescript-support)
  * [Browser Usage](#browser-usage)
  * [Troubleshooting](#troubleshooting)
  * [Support](#support)
  * [SDK Installation](#sdk-installation)
  * [Requirements](#requirements)
  * [SDK Example Usage](#sdk-example-usage)
  * [Authentication](#authentication)
  * [Available Resources and Operations](#available-resources-and-operations)
  * [Standalone functions](#standalone-functions)
  * [File uploads](#file-uploads)
  * [Retries](#retries)
  * [Error Handling](#error-handling-1)
  * [Custom HTTP Client](#custom-http-client)
  * [Debugging](#debugging)

<!-- End Table of Contents [toc] -->

<!-- Start SDK Installation [installation] -->
## SDK Installation

The SDK can be installed with either [npm](https://www.npmjs.com/), [pnpm](https://pnpm.io/), [bun](https://bun.sh/) or [yarn](https://classic.yarnpkg.com/en/) package managers.

### NPM

```bash
npm add flexprice-sdk-test
```

### PNPM

```bash
pnpm add flexprice-sdk-test
```

### Bun

```bash
bun add flexprice-sdk-test
```

### Yarn

```bash
yarn add flexprice-sdk-test
```
<!-- End SDK Installation [installation] -->

<!-- Start Requirements [requirements] -->
## Requirements

For supported JavaScript runtimes, please consult [RUNTIMES.md](RUNTIMES.md).
<!-- End Requirements [requirements] -->

<!-- Start SDK Example Usage [usage] -->
## SDK Example Usage

### Example

```typescript
import { FlexPrice } from "flexprice-sdk-test";

const flexPrice = new FlexPrice({
  serverURL: "https://api.example.com",
  apiKeyAuth: "<YOUR_API_KEY_HERE>",
});

async function run() {
  const result = await flexPrice.addons.getAddons({});

  console.log(result);
}

run();

```
<!-- End SDK Example Usage [usage] -->

<!-- Start Authentication [security] -->
## Authentication

### Per-Client Security Schemes

This SDK supports the following security scheme globally:

| Name         | Type   | Scheme  |
| ------------ | ------ | ------- |
| `apiKeyAuth` | apiKey | API key |

To authenticate with the API the `apiKeyAuth` parameter must be set when initializing the SDK client instance. For example:
```typescript
import { FlexPrice } from "flexprice-sdk-test";

const flexPrice = new FlexPrice({
  serverURL: "https://api.example.com",
  apiKeyAuth: "<YOUR_API_KEY_HERE>",
});

async function run() {
  const result = await flexPrice.addons.getAddons({});

  console.log(result);
}

run();

```
<!-- End Authentication [security] -->

<!-- Start Available Resources and Operations [operations] -->
## Available Resources and Operations

<details open>
<summary>Available methods</summary>

### [Addons](docs/sdks/addons/README.md)

* [getAddons](docs/sdks/addons/README.md#getaddons) - List addons
* [postAddons](docs/sdks/addons/README.md#postaddons) - Create addon
* [getAddonsLookupLookupKey](docs/sdks/addons/README.md#getaddonslookuplookupkey) - Get addon by lookup key
* [postAddonsSearch](docs/sdks/addons/README.md#postaddonssearch) - List addons by filter
* [getAddonsId](docs/sdks/addons/README.md#getaddonsid) - Get addon
* [putAddonsId](docs/sdks/addons/README.md#putaddonsid) - Update addon
* [deleteAddonsId](docs/sdks/addons/README.md#deleteaddonsid) - Delete addon

### [AlertLogs](docs/sdks/alertlogs/README.md)

* [postAlertSearch](docs/sdks/alertlogs/README.md#postalertsearch) - List alert logs by filter

### [Auth](docs/sdks/auth/README.md)

* [postAuthLogin](docs/sdks/auth/README.md#postauthlogin) - Login
* [postAuthSignup](docs/sdks/auth/README.md#postauthsignup) - Sign up

### [Connections](docs/sdks/connections/README.md)

* [getConnections](docs/sdks/connections/README.md#getconnections) - Get connections
* [postConnectionsSearch](docs/sdks/connections/README.md#postconnectionssearch) - List connections by filter
* [getConnectionsId](docs/sdks/connections/README.md#getconnectionsid) - Get a connection
* [putConnectionsId](docs/sdks/connections/README.md#putconnectionsid) - Update a connection
* [deleteConnectionsId](docs/sdks/connections/README.md#deleteconnectionsid) - Delete a connection

### [Costs](docs/sdks/costs/README.md)

* [postCosts](docs/sdks/costs/README.md#postcosts) - Create a new costsheet
* [getCostsActive](docs/sdks/costs/README.md#getcostsactive) - Get active costsheet for tenant
* [postCostsAnalytics](docs/sdks/costs/README.md#postcostsanalytics) - Get combined revenue and cost analytics
* [postCostsAnalyticsV2](docs/sdks/costs/README.md#postcostsanalyticsv2) - Get combined revenue and cost analytics
* [postCostsSearch](docs/sdks/costs/README.md#postcostssearch) - List costsheets by filter
* [getCostsId](docs/sdks/costs/README.md#getcostsid) - Get a costsheet by ID
* [putCostsId](docs/sdks/costs/README.md#putcostsid) - Update a costsheet
* [deleteCostsId](docs/sdks/costs/README.md#deletecostsid) - Delete a costsheet

### [Coupons](docs/sdks/coupons/README.md)

* [getCoupons](docs/sdks/coupons/README.md#getcoupons) - List coupons with filtering
* [postCoupons](docs/sdks/coupons/README.md#postcoupons) - Create a new coupon
* [getCouponsId](docs/sdks/coupons/README.md#getcouponsid) - Get a coupon by ID
* [putCouponsId](docs/sdks/coupons/README.md#putcouponsid) - Update a coupon
* [deleteCouponsId](docs/sdks/coupons/README.md#deletecouponsid) - Delete a coupon

### [CreditNotes](docs/sdks/creditnotes/README.md)

* [getCreditnotes](docs/sdks/creditnotes/README.md#getcreditnotes) - List credit notes with filtering
* [postCreditnotes](docs/sdks/creditnotes/README.md#postcreditnotes) - Create a new credit note
* [getCreditnotesId](docs/sdks/creditnotes/README.md#getcreditnotesid) - Get a credit note by ID
* [postCreditnotesIdFinalize](docs/sdks/creditnotes/README.md#postcreditnotesidfinalize) - Process a draft credit note
* [postCreditnotesIdVoid](docs/sdks/creditnotes/README.md#postcreditnotesidvoid) - Void a credit note

### [CreditGrants](docs/sdks/creditgrants/README.md)

* [getCreditgrants](docs/sdks/creditgrants/README.md#getcreditgrants) - Get credit grants
* [postCreditgrants](docs/sdks/creditgrants/README.md#postcreditgrants) - Create a new credit grant
* [getCreditgrantsId](docs/sdks/creditgrants/README.md#getcreditgrantsid) - Get a credit grant by ID
* [putCreditgrantsId](docs/sdks/creditgrants/README.md#putcreditgrantsid) - Update a credit grant
* [deleteCreditgrantsId](docs/sdks/creditgrants/README.md#deletecreditgrantsid) - Delete a credit grant
* [getPlansIdCreditgrants](docs/sdks/creditgrants/README.md#getplansidcreditgrants) - Get plan credit grants

### [Customers](docs/sdks/customers/README.md)

* [getCustomers](docs/sdks/customers/README.md#getcustomers) - Get customers
* [postCustomers](docs/sdks/customers/README.md#postcustomers) - Create a customer
* [getCustomersExternalExternalId](docs/sdks/customers/README.md#getcustomersexternalexternalid) - Get a customer by external id
* [postCustomersSearch](docs/sdks/customers/README.md#postcustomerssearch) - List customers by filter
* [getCustomersUsage](docs/sdks/customers/README.md#getcustomersusage) - Get customer usage summary
* [getCustomersId](docs/sdks/customers/README.md#getcustomersid) - Get a customer
* [putCustomersId](docs/sdks/customers/README.md#putcustomersid) - Update a customer
* [deleteCustomersId](docs/sdks/customers/README.md#deletecustomersid) - Delete a customer
* [getCustomersIdEntitlements](docs/sdks/customers/README.md#getcustomersidentitlements) - Get customer entitlements
* [getCustomersIdGrantsUpcoming](docs/sdks/customers/README.md#getcustomersidgrantsupcoming) - Get upcoming credit grant applications

### [Entitlements](docs/sdks/entitlements/README.md)

* [getAddonsIdEntitlements](docs/sdks/entitlements/README.md#getaddonsidentitlements) - Get addon entitlements
* [getEntitlements](docs/sdks/entitlements/README.md#getentitlements) - Get entitlements
* [postEntitlements](docs/sdks/entitlements/README.md#postentitlements) - Create a new entitlement
* [postEntitlementsBulk](docs/sdks/entitlements/README.md#postentitlementsbulk) - Create multiple entitlements in bulk
* [postEntitlementsSearch](docs/sdks/entitlements/README.md#postentitlementssearch) - List entitlements by filter
* [getEntitlementsId](docs/sdks/entitlements/README.md#getentitlementsid) - Get an entitlement by ID
* [putEntitlementsId](docs/sdks/entitlements/README.md#putentitlementsid) - Update an entitlement
* [deleteEntitlementsId](docs/sdks/entitlements/README.md#deleteentitlementsid) - Delete an entitlement
* [getPlansIdEntitlements](docs/sdks/entitlements/README.md#getplansidentitlements) - Get plan entitlements

### [EntityIntegrationMappings](docs/sdks/entityintegrationmappings/README.md)

* [getEntityIntegrationMappings](docs/sdks/entityintegrationmappings/README.md#getentityintegrationmappings) - List entity integration mappings
* [postEntityIntegrationMappings](docs/sdks/entityintegrationmappings/README.md#postentityintegrationmappings) - Create entity integration mapping
* [getEntityIntegrationMappingsId](docs/sdks/entityintegrationmappings/README.md#getentityintegrationmappingsid) - Get entity integration mapping
* [deleteEntityIntegrationMappingsId](docs/sdks/entityintegrationmappings/README.md#deleteentityintegrationmappingsid) - Delete entity integration mapping

### [Environments](docs/sdks/environments/README.md)

* [getEnvironments](docs/sdks/environments/README.md#getenvironments) - Get environments
* [postEnvironments](docs/sdks/environments/README.md#postenvironments) - Create an environment
* [getEnvironmentsId](docs/sdks/environments/README.md#getenvironmentsid) - Get an environment
* [putEnvironmentsId](docs/sdks/environments/README.md#putenvironmentsid) - Update an environment

### [Events](docs/sdks/events/README.md)

* [postEvents](docs/sdks/events/README.md#postevents) - Ingest event
* [postEventsAnalytics](docs/sdks/events/README.md#posteventsanalytics) - Get usage analytics
* [postEventsBulk](docs/sdks/events/README.md#posteventsbulk) - Bulk Ingest events
* [postEventsHuggingfaceInference](docs/sdks/events/README.md#posteventshuggingfaceinference) - Get hugging face inference data
* [getEventsMonitoring](docs/sdks/events/README.md#geteventsmonitoring) - Get monitoring data
* [postEventsQuery](docs/sdks/events/README.md#posteventsquery) - List raw events
* [postEventsUsage](docs/sdks/events/README.md#posteventsusage) - Get usage statistics
* [postEventsUsageMeter](docs/sdks/events/README.md#posteventsusagemeter) - Get usage by meter

### [Features](docs/sdks/features/README.md)

* [getFeatures](docs/sdks/features/README.md#getfeatures) - List features
* [postFeatures](docs/sdks/features/README.md#postfeatures) - Create a new feature
* [postFeaturesSearch](docs/sdks/features/README.md#postfeaturessearch) - List features by filter
* [getFeaturesId](docs/sdks/features/README.md#getfeaturesid) - Get a feature by ID
* [putFeaturesId](docs/sdks/features/README.md#putfeaturesid) - Update a feature
* [deleteFeaturesId](docs/sdks/features/README.md#deletefeaturesid) - Delete a feature

### [Groups](docs/sdks/groups/README.md)

* [postGroups](docs/sdks/groups/README.md#postgroups) - Create a group
* [postGroupsSearch](docs/sdks/groups/README.md#postgroupssearch) - Get groups
* [getGroupsId](docs/sdks/groups/README.md#getgroupsid) - Get a group
* [deleteGroupsId](docs/sdks/groups/README.md#deletegroupsid) - Delete a group

### [Integrations](docs/sdks/integrations/README.md)

* [getSecretsIntegrationsByProviderProvider](docs/sdks/integrations/README.md#getsecretsintegrationsbyproviderprovider) - Get integration details
* [postSecretsIntegrationsCreateProvider](docs/sdks/integrations/README.md#postsecretsintegrationscreateprovider) - Create or update an integration
* [getSecretsIntegrationsLinked](docs/sdks/integrations/README.md#getsecretsintegrationslinked) - List linked integrations
* [deleteSecretsIntegrationsId](docs/sdks/integrations/README.md#deletesecretsintegrationsid) - Delete an integration

### [Invoices](docs/sdks/invoices/README.md)

* [getCustomersIdInvoicesSummary](docs/sdks/invoices/README.md#getcustomersidinvoicessummary) - Get a customer invoice summary
* [getInvoices](docs/sdks/invoices/README.md#getinvoices) - List invoices
* [postInvoices](docs/sdks/invoices/README.md#postinvoices) - Create a new one off invoice
* [postInvoicesPreview](docs/sdks/invoices/README.md#postinvoicespreview) - Get a preview invoice
* [postInvoicesSearch](docs/sdks/invoices/README.md#postinvoicessearch) - List invoices by filter
* [getInvoicesId](docs/sdks/invoices/README.md#getinvoicesid) - Get an invoice by ID
* [putInvoicesId](docs/sdks/invoices/README.md#putinvoicesid) - Update an invoice
* [postInvoicesIdCommsTrigger](docs/sdks/invoices/README.md#postinvoicesidcommstrigger) - Trigger communication webhook for an invoice
* [postInvoicesIdFinalize](docs/sdks/invoices/README.md#postinvoicesidfinalize) - Finalize an invoice
* [putInvoicesIdPayment](docs/sdks/invoices/README.md#putinvoicesidpayment) - Update invoice payment status
* [postInvoicesIdPaymentAttempt](docs/sdks/invoices/README.md#postinvoicesidpaymentattempt) - Attempt payment for an invoice
* [getInvoicesIdPdf](docs/sdks/invoices/README.md#getinvoicesidpdf) - Get PDF for an invoice
* [postInvoicesIdRecalculate](docs/sdks/invoices/README.md#postinvoicesidrecalculate) - Recalculate invoice totals and line items
* [postInvoicesIdVoid](docs/sdks/invoices/README.md#postinvoicesidvoid) - Void an invoice

### [Payments](docs/sdks/payments/README.md)

* [getPayments](docs/sdks/payments/README.md#getpayments) - List payments
* [postPayments](docs/sdks/payments/README.md#postpayments) - Create a new payment
* [getPaymentsId](docs/sdks/payments/README.md#getpaymentsid) - Get a payment by ID
* [putPaymentsId](docs/sdks/payments/README.md#putpaymentsid) - Update a payment
* [deletePaymentsId](docs/sdks/payments/README.md#deletepaymentsid) - Delete a payment
* [postPaymentsIdProcess](docs/sdks/payments/README.md#postpaymentsidprocess) - Process a payment

### [Plans](docs/sdks/plans/README.md)

* [getPlans](docs/sdks/plans/README.md#getplans) - Get plans
* [postPlans](docs/sdks/plans/README.md#postplans) - Create a new plan
* [postPlansSearch](docs/sdks/plans/README.md#postplanssearch) - List plans by filter
* [getPlansId](docs/sdks/plans/README.md#getplansid) - Get a plan
* [putPlansId](docs/sdks/plans/README.md#putplansid) - Update a plan
* [deletePlansId](docs/sdks/plans/README.md#deleteplansid) - Delete a plan
* [postPlansIdSyncSubscriptions](docs/sdks/plans/README.md#postplansidsyncsubscriptions) - Synchronize plan prices

### [PriceUnits](docs/sdks/priceunits/README.md)

* [getPricesUnits](docs/sdks/priceunits/README.md#getpricesunits) - List price units
* [postPricesUnits](docs/sdks/priceunits/README.md#postpricesunits) - Create a new price unit
* [getPricesUnitsCodeCode](docs/sdks/priceunits/README.md#getpricesunitscodecode) - Get a price unit by code
* [postPricesUnitsSearch](docs/sdks/priceunits/README.md#postpricesunitssearch) - List price units by filter
* [getPricesUnitsId](docs/sdks/priceunits/README.md#getpricesunitsid) - Get a price unit by ID
* [putPricesUnitsId](docs/sdks/priceunits/README.md#putpricesunitsid) - Update a price unit
* [deletePricesUnitsId](docs/sdks/priceunits/README.md#deletepricesunitsid) - Delete a price unit

### [Prices](docs/sdks/prices/README.md)

* [getPrices](docs/sdks/prices/README.md#getprices) - Get prices
* [postPrices](docs/sdks/prices/README.md#postprices) - Create a new price
* [postPricesBulk](docs/sdks/prices/README.md#postpricesbulk) - Create multiple prices in bulk
* [getPricesId](docs/sdks/prices/README.md#getpricesid) - Get a price by ID
* [putPricesId](docs/sdks/prices/README.md#putpricesid) - Update a price
* [deletePricesId](docs/sdks/prices/README.md#deletepricesid) - Delete a price

### [Rbac](docs/sdks/rbac/README.md)

* [getRbacRoles](docs/sdks/rbac/README.md#getrbacroles) - List all RBAC roles
* [getRbacRolesId](docs/sdks/rbac/README.md#getrbacrolesid) - Get a specific RBAC role

### [ScheduledTasks](docs/sdks/scheduledtasks/README.md)

* [getTasksScheduled](docs/sdks/scheduledtasks/README.md#gettasksscheduled) - List scheduled tasks
* [postTasksScheduled](docs/sdks/scheduledtasks/README.md#posttasksscheduled) - Create a scheduled task
* [postTasksScheduledScheduleUpdateBillingPeriod](docs/sdks/scheduledtasks/README.md#posttasksscheduledscheduleupdatebillingperiod) - Schedule update billing period
* [getTasksScheduledId](docs/sdks/scheduledtasks/README.md#gettasksscheduledid) - Get a scheduled task
* [putTasksScheduledId](docs/sdks/scheduledtasks/README.md#puttasksscheduledid) - Update a scheduled task
* [deleteTasksScheduledId](docs/sdks/scheduledtasks/README.md#deletetasksscheduledid) - Delete a scheduled task
* [postTasksScheduledIdRun](docs/sdks/scheduledtasks/README.md#posttasksscheduledidrun) - Trigger force run

### [Secrets](docs/sdks/secrets/README.md)

* [getSecretsApiKeys](docs/sdks/secrets/README.md#getsecretsapikeys) - List API keys
* [postSecretsApiKeys](docs/sdks/secrets/README.md#postsecretsapikeys) - Create a new API key
* [deleteSecretsApiKeysId](docs/sdks/secrets/README.md#deletesecretsapikeysid) - Delete an API key

### [Subscriptions](docs/sdks/subscriptions/README.md)

* [getSubscriptions](docs/sdks/subscriptions/README.md#getsubscriptions) - List subscriptions
* [postSubscriptions](docs/sdks/subscriptions/README.md#postsubscriptions) - Create subscription
* [postSubscriptionsAddon](docs/sdks/subscriptions/README.md#postsubscriptionsaddon) - Add addon to subscription
* [deleteSubscriptionsAddon](docs/sdks/subscriptions/README.md#deletesubscriptionsaddon) - Remove addon from subscription
* [putSubscriptionsLineitemsId](docs/sdks/subscriptions/README.md#putsubscriptionslineitemsid) - Update subscription line item
* [deleteSubscriptionsLineitemsId](docs/sdks/subscriptions/README.md#deletesubscriptionslineitemsid) - Delete subscription line item
* [postSubscriptionsSearch](docs/sdks/subscriptions/README.md#postsubscriptionssearch) - List subscriptions by filter
* [postSubscriptionsUsage](docs/sdks/subscriptions/README.md#postsubscriptionsusage) - Get usage by subscription
* [getSubscriptionsId](docs/sdks/subscriptions/README.md#getsubscriptionsid) - Get subscription
* [postSubscriptionsIdActivate](docs/sdks/subscriptions/README.md#postsubscriptionsidactivate) - Activate draft subscription
* [getSubscriptionsIdAddonsAssociations](docs/sdks/subscriptions/README.md#getsubscriptionsidaddonsassociations) - Get active addon associations
* [postSubscriptionsIdCancel](docs/sdks/subscriptions/README.md#postsubscriptionsidcancel) - Cancel subscription
* [postSubscriptionsIdChangeExecute](docs/sdks/subscriptions/README.md#postsubscriptionsidchangeexecute) - Execute subscription plan change
* [postSubscriptionsIdChangePreview](docs/sdks/subscriptions/README.md#postsubscriptionsidchangepreview) - Preview subscription plan change
* [getSubscriptionsIdEntitlements](docs/sdks/subscriptions/README.md#getsubscriptionsidentitlements) - Get subscription entitlements
* [getSubscriptionsIdGrantsUpcoming](docs/sdks/subscriptions/README.md#getsubscriptionsidgrantsupcoming) - Get upcoming credit grant applications
* [postSubscriptionsIdPause](docs/sdks/subscriptions/README.md#postsubscriptionsidpause) - Pause a subscription
* [getSubscriptionsIdPauses](docs/sdks/subscriptions/README.md#getsubscriptionsidpauses) - List all pauses for a subscription
* [postSubscriptionsIdResume](docs/sdks/subscriptions/README.md#postsubscriptionsidresume) - Resume a paused subscription

### [Tasks](docs/sdks/tasks/README.md)

* [getTasks](docs/sdks/tasks/README.md#gettasks) - List tasks
* [postTasks](docs/sdks/tasks/README.md#posttasks) - Create a new task
* [getTasksResult](docs/sdks/tasks/README.md#gettasksresult) - Get task processing result
* [getTasksId](docs/sdks/tasks/README.md#gettasksid) - Get a task
* [putTasksIdStatus](docs/sdks/tasks/README.md#puttasksidstatus) - Update task status

### [TaxAssociations](docs/sdks/taxassociations/README.md)

* [getTaxesAssociations](docs/sdks/taxassociations/README.md#gettaxesassociations) - List tax associations
* [postTaxesAssociations](docs/sdks/taxassociations/README.md#posttaxesassociations) - Create Tax Association
* [getTaxesAssociationsId](docs/sdks/taxassociations/README.md#gettaxesassociationsid) - Get Tax Association
* [putTaxesAssociationsId](docs/sdks/taxassociations/README.md#puttaxesassociationsid) - Update tax association
* [deleteTaxesAssociationsId](docs/sdks/taxassociations/README.md#deletetaxesassociationsid) - Delete tax association

### [TaxRates](docs/sdks/taxrates/README.md)

* [getTaxesRates](docs/sdks/taxrates/README.md#gettaxesrates) - Get tax rates
* [postTaxesRates](docs/sdks/taxrates/README.md#posttaxesrates) - Create a tax rate
* [getTaxesRatesId](docs/sdks/taxrates/README.md#gettaxesratesid) - Get a tax rate
* [putTaxesRatesId](docs/sdks/taxrates/README.md#puttaxesratesid) - Update a tax rate
* [deleteTaxesRatesId](docs/sdks/taxrates/README.md#deletetaxesratesid) - Delete a tax rate

### [Tenants](docs/sdks/tenants/README.md)

* [getTenantBilling](docs/sdks/tenants/README.md#gettenantbilling) - Get billing usage for the current tenant
* [postTenants](docs/sdks/tenants/README.md#posttenants) - Create a new tenant
* [putTenantsUpdate](docs/sdks/tenants/README.md#puttenantsupdate) - Update a tenant
* [getTenantsId](docs/sdks/tenants/README.md#gettenantsid) - Get tenant by ID

### [Users](docs/sdks/users/README.md)

* [postUsers](docs/sdks/users/README.md#postusers) - Create service account
* [getUsersMe](docs/sdks/users/README.md#getusersme) - Get user info
* [postUsersSearch](docs/sdks/users/README.md#postuserssearch) - List users with filters

### [Wallets](docs/sdks/wallets/README.md)

* [getCustomersWallets](docs/sdks/wallets/README.md#getcustomerswallets) - Get Customer Wallets
* [getCustomersIdWallets](docs/sdks/wallets/README.md#getcustomersidwallets) - Get wallets by customer ID
* [getWallets](docs/sdks/wallets/README.md#getwallets) - List wallets
* [postWallets](docs/sdks/wallets/README.md#postwallets) - Create a new wallet
* [postWalletsSearch](docs/sdks/wallets/README.md#postwalletssearch) - List wallets by filter
* [postWalletsTransactionsSearch](docs/sdks/wallets/README.md#postwalletstransactionssearch) - List wallet transactions by filter
* [getWalletsId](docs/sdks/wallets/README.md#getwalletsid) - Get wallet by ID
* [putWalletsId](docs/sdks/wallets/README.md#putwalletsid) - Update a wallet
* [getWalletsIdBalanceRealTime](docs/sdks/wallets/README.md#getwalletsidbalancerealtime) - Get wallet balance
* [postWalletsIdTerminate](docs/sdks/wallets/README.md#postwalletsidterminate) - Terminate a wallet
* [postWalletsIdTopUp](docs/sdks/wallets/README.md#postwalletsidtopup) - Top up wallet
* [getWalletsIdTransactions](docs/sdks/wallets/README.md#getwalletsidtransactions) - Get wallet transactions

### [Webhooks](docs/sdks/webhooks/README.md)

* [postWebhooksChargebeeTenantIdEnvironmentId](docs/sdks/webhooks/README.md#postwebhookschargebeetenantidenvironmentid) - Handle Chargebee webhook events
* [postWebhooksHubspotTenantIdEnvironmentId](docs/sdks/webhooks/README.md#postwebhookshubspottenantidenvironmentid) - Handle HubSpot webhook events
* [postWebhooksNomodTenantIdEnvironmentId](docs/sdks/webhooks/README.md#postwebhooksnomodtenantidenvironmentid) - Handle Nomod webhook events
* [postWebhooksQuickbooksTenantIdEnvironmentId](docs/sdks/webhooks/README.md#postwebhooksquickbookstenantidenvironmentid) - Handle QuickBooks webhook events
* [postWebhooksRazorpayTenantIdEnvironmentId](docs/sdks/webhooks/README.md#postwebhooksrazorpaytenantidenvironmentid) - Handle Razorpay webhook events
* [postWebhooksStripeTenantIdEnvironmentId](docs/sdks/webhooks/README.md#postwebhooksstripetenantidenvironmentid) - Handle Stripe webhook events

</details>
<!-- End Available Resources and Operations [operations] -->

<!-- Start Standalone functions [standalone-funcs] -->
## Standalone functions

All the methods listed above are available as standalone functions. These
functions are ideal for use in applications running in the browser, serverless
runtimes or other environments where application bundle size is a primary
concern. When using a bundler to build your application, all unused
functionality will be either excluded from the final bundle or tree-shaken away.

To read more about standalone functions, check [FUNCTIONS.md](./FUNCTIONS.md).

<details>

<summary>Available standalone functions</summary>

- [`addonsDeleteAddonsId`](docs/sdks/addons/README.md#deleteaddonsid) - Delete addon
- [`addonsGetAddons`](docs/sdks/addons/README.md#getaddons) - List addons
- [`addonsGetAddonsId`](docs/sdks/addons/README.md#getaddonsid) - Get addon
- [`addonsGetAddonsLookupLookupKey`](docs/sdks/addons/README.md#getaddonslookuplookupkey) - Get addon by lookup key
- [`addonsPostAddons`](docs/sdks/addons/README.md#postaddons) - Create addon
- [`addonsPostAddonsSearch`](docs/sdks/addons/README.md#postaddonssearch) - List addons by filter
- [`addonsPutAddonsId`](docs/sdks/addons/README.md#putaddonsid) - Update addon
- [`alertLogsPostAlertSearch`](docs/sdks/alertlogs/README.md#postalertsearch) - List alert logs by filter
- [`authPostAuthLogin`](docs/sdks/auth/README.md#postauthlogin) - Login
- [`authPostAuthSignup`](docs/sdks/auth/README.md#postauthsignup) - Sign up
- [`connectionsDeleteConnectionsId`](docs/sdks/connections/README.md#deleteconnectionsid) - Delete a connection
- [`connectionsGetConnections`](docs/sdks/connections/README.md#getconnections) - Get connections
- [`connectionsGetConnectionsId`](docs/sdks/connections/README.md#getconnectionsid) - Get a connection
- [`connectionsPostConnectionsSearch`](docs/sdks/connections/README.md#postconnectionssearch) - List connections by filter
- [`connectionsPutConnectionsId`](docs/sdks/connections/README.md#putconnectionsid) - Update a connection
- [`costsDeleteCostsId`](docs/sdks/costs/README.md#deletecostsid) - Delete a costsheet
- [`costsGetCostsActive`](docs/sdks/costs/README.md#getcostsactive) - Get active costsheet for tenant
- [`costsGetCostsId`](docs/sdks/costs/README.md#getcostsid) - Get a costsheet by ID
- [`costsPostCosts`](docs/sdks/costs/README.md#postcosts) - Create a new costsheet
- [`costsPostCostsAnalytics`](docs/sdks/costs/README.md#postcostsanalytics) - Get combined revenue and cost analytics
- [`costsPostCostsAnalyticsV2`](docs/sdks/costs/README.md#postcostsanalyticsv2) - Get combined revenue and cost analytics
- [`costsPostCostsSearch`](docs/sdks/costs/README.md#postcostssearch) - List costsheets by filter
- [`costsPutCostsId`](docs/sdks/costs/README.md#putcostsid) - Update a costsheet
- [`couponsDeleteCouponsId`](docs/sdks/coupons/README.md#deletecouponsid) - Delete a coupon
- [`couponsGetCoupons`](docs/sdks/coupons/README.md#getcoupons) - List coupons with filtering
- [`couponsGetCouponsId`](docs/sdks/coupons/README.md#getcouponsid) - Get a coupon by ID
- [`couponsPostCoupons`](docs/sdks/coupons/README.md#postcoupons) - Create a new coupon
- [`couponsPutCouponsId`](docs/sdks/coupons/README.md#putcouponsid) - Update a coupon
- [`creditGrantsDeleteCreditgrantsId`](docs/sdks/creditgrants/README.md#deletecreditgrantsid) - Delete a credit grant
- [`creditGrantsGetCreditgrants`](docs/sdks/creditgrants/README.md#getcreditgrants) - Get credit grants
- [`creditGrantsGetCreditgrantsId`](docs/sdks/creditgrants/README.md#getcreditgrantsid) - Get a credit grant by ID
- [`creditGrantsGetPlansIdCreditgrants`](docs/sdks/creditgrants/README.md#getplansidcreditgrants) - Get plan credit grants
- [`creditGrantsPostCreditgrants`](docs/sdks/creditgrants/README.md#postcreditgrants) - Create a new credit grant
- [`creditGrantsPutCreditgrantsId`](docs/sdks/creditgrants/README.md#putcreditgrantsid) - Update a credit grant
- [`creditNotesGetCreditnotes`](docs/sdks/creditnotes/README.md#getcreditnotes) - List credit notes with filtering
- [`creditNotesGetCreditnotesId`](docs/sdks/creditnotes/README.md#getcreditnotesid) - Get a credit note by ID
- [`creditNotesPostCreditnotes`](docs/sdks/creditnotes/README.md#postcreditnotes) - Create a new credit note
- [`creditNotesPostCreditnotesIdFinalize`](docs/sdks/creditnotes/README.md#postcreditnotesidfinalize) - Process a draft credit note
- [`creditNotesPostCreditnotesIdVoid`](docs/sdks/creditnotes/README.md#postcreditnotesidvoid) - Void a credit note
- [`customersDeleteCustomersId`](docs/sdks/customers/README.md#deletecustomersid) - Delete a customer
- [`customersGetCustomers`](docs/sdks/customers/README.md#getcustomers) - Get customers
- [`customersGetCustomersExternalExternalId`](docs/sdks/customers/README.md#getcustomersexternalexternalid) - Get a customer by external id
- [`customersGetCustomersId`](docs/sdks/customers/README.md#getcustomersid) - Get a customer
- [`customersGetCustomersIdEntitlements`](docs/sdks/customers/README.md#getcustomersidentitlements) - Get customer entitlements
- [`customersGetCustomersIdGrantsUpcoming`](docs/sdks/customers/README.md#getcustomersidgrantsupcoming) - Get upcoming credit grant applications
- [`customersGetCustomersUsage`](docs/sdks/customers/README.md#getcustomersusage) - Get customer usage summary
- [`customersPostCustomers`](docs/sdks/customers/README.md#postcustomers) - Create a customer
- [`customersPostCustomersSearch`](docs/sdks/customers/README.md#postcustomerssearch) - List customers by filter
- [`customersPutCustomersId`](docs/sdks/customers/README.md#putcustomersid) - Update a customer
- [`entitlementsDeleteEntitlementsId`](docs/sdks/entitlements/README.md#deleteentitlementsid) - Delete an entitlement
- [`entitlementsGetAddonsIdEntitlements`](docs/sdks/entitlements/README.md#getaddonsidentitlements) - Get addon entitlements
- [`entitlementsGetEntitlements`](docs/sdks/entitlements/README.md#getentitlements) - Get entitlements
- [`entitlementsGetEntitlementsId`](docs/sdks/entitlements/README.md#getentitlementsid) - Get an entitlement by ID
- [`entitlementsGetPlansIdEntitlements`](docs/sdks/entitlements/README.md#getplansidentitlements) - Get plan entitlements
- [`entitlementsPostEntitlements`](docs/sdks/entitlements/README.md#postentitlements) - Create a new entitlement
- [`entitlementsPostEntitlementsBulk`](docs/sdks/entitlements/README.md#postentitlementsbulk) - Create multiple entitlements in bulk
- [`entitlementsPostEntitlementsSearch`](docs/sdks/entitlements/README.md#postentitlementssearch) - List entitlements by filter
- [`entitlementsPutEntitlementsId`](docs/sdks/entitlements/README.md#putentitlementsid) - Update an entitlement
- [`entityIntegrationMappingsDeleteEntityIntegrationMappingsId`](docs/sdks/entityintegrationmappings/README.md#deleteentityintegrationmappingsid) - Delete entity integration mapping
- [`entityIntegrationMappingsGetEntityIntegrationMappings`](docs/sdks/entityintegrationmappings/README.md#getentityintegrationmappings) - List entity integration mappings
- [`entityIntegrationMappingsGetEntityIntegrationMappingsId`](docs/sdks/entityintegrationmappings/README.md#getentityintegrationmappingsid) - Get entity integration mapping
- [`entityIntegrationMappingsPostEntityIntegrationMappings`](docs/sdks/entityintegrationmappings/README.md#postentityintegrationmappings) - Create entity integration mapping
- [`environmentsGetEnvironments`](docs/sdks/environments/README.md#getenvironments) - Get environments
- [`environmentsGetEnvironmentsId`](docs/sdks/environments/README.md#getenvironmentsid) - Get an environment
- [`environmentsPostEnvironments`](docs/sdks/environments/README.md#postenvironments) - Create an environment
- [`environmentsPutEnvironmentsId`](docs/sdks/environments/README.md#putenvironmentsid) - Update an environment
- [`eventsGetEventsMonitoring`](docs/sdks/events/README.md#geteventsmonitoring) - Get monitoring data
- [`eventsPostEvents`](docs/sdks/events/README.md#postevents) - Ingest event
- [`eventsPostEventsAnalytics`](docs/sdks/events/README.md#posteventsanalytics) - Get usage analytics
- [`eventsPostEventsBulk`](docs/sdks/events/README.md#posteventsbulk) - Bulk Ingest events
- [`eventsPostEventsHuggingfaceInference`](docs/sdks/events/README.md#posteventshuggingfaceinference) - Get hugging face inference data
- [`eventsPostEventsQuery`](docs/sdks/events/README.md#posteventsquery) - List raw events
- [`eventsPostEventsUsage`](docs/sdks/events/README.md#posteventsusage) - Get usage statistics
- [`eventsPostEventsUsageMeter`](docs/sdks/events/README.md#posteventsusagemeter) - Get usage by meter
- [`featuresDeleteFeaturesId`](docs/sdks/features/README.md#deletefeaturesid) - Delete a feature
- [`featuresGetFeatures`](docs/sdks/features/README.md#getfeatures) - List features
- [`featuresGetFeaturesId`](docs/sdks/features/README.md#getfeaturesid) - Get a feature by ID
- [`featuresPostFeatures`](docs/sdks/features/README.md#postfeatures) - Create a new feature
- [`featuresPostFeaturesSearch`](docs/sdks/features/README.md#postfeaturessearch) - List features by filter
- [`featuresPutFeaturesId`](docs/sdks/features/README.md#putfeaturesid) - Update a feature
- [`groupsDeleteGroupsId`](docs/sdks/groups/README.md#deletegroupsid) - Delete a group
- [`groupsGetGroupsId`](docs/sdks/groups/README.md#getgroupsid) - Get a group
- [`groupsPostGroups`](docs/sdks/groups/README.md#postgroups) - Create a group
- [`groupsPostGroupsSearch`](docs/sdks/groups/README.md#postgroupssearch) - Get groups
- [`integrationsDeleteSecretsIntegrationsId`](docs/sdks/integrations/README.md#deletesecretsintegrationsid) - Delete an integration
- [`integrationsGetSecretsIntegrationsByProviderProvider`](docs/sdks/integrations/README.md#getsecretsintegrationsbyproviderprovider) - Get integration details
- [`integrationsGetSecretsIntegrationsLinked`](docs/sdks/integrations/README.md#getsecretsintegrationslinked) - List linked integrations
- [`integrationsPostSecretsIntegrationsCreateProvider`](docs/sdks/integrations/README.md#postsecretsintegrationscreateprovider) - Create or update an integration
- [`invoicesGetCustomersIdInvoicesSummary`](docs/sdks/invoices/README.md#getcustomersidinvoicessummary) - Get a customer invoice summary
- [`invoicesGetInvoices`](docs/sdks/invoices/README.md#getinvoices) - List invoices
- [`invoicesGetInvoicesId`](docs/sdks/invoices/README.md#getinvoicesid) - Get an invoice by ID
- [`invoicesGetInvoicesIdPdf`](docs/sdks/invoices/README.md#getinvoicesidpdf) - Get PDF for an invoice
- [`invoicesPostInvoices`](docs/sdks/invoices/README.md#postinvoices) - Create a new one off invoice
- [`invoicesPostInvoicesIdCommsTrigger`](docs/sdks/invoices/README.md#postinvoicesidcommstrigger) - Trigger communication webhook for an invoice
- [`invoicesPostInvoicesIdFinalize`](docs/sdks/invoices/README.md#postinvoicesidfinalize) - Finalize an invoice
- [`invoicesPostInvoicesIdPaymentAttempt`](docs/sdks/invoices/README.md#postinvoicesidpaymentattempt) - Attempt payment for an invoice
- [`invoicesPostInvoicesIdRecalculate`](docs/sdks/invoices/README.md#postinvoicesidrecalculate) - Recalculate invoice totals and line items
- [`invoicesPostInvoicesIdVoid`](docs/sdks/invoices/README.md#postinvoicesidvoid) - Void an invoice
- [`invoicesPostInvoicesPreview`](docs/sdks/invoices/README.md#postinvoicespreview) - Get a preview invoice
- [`invoicesPostInvoicesSearch`](docs/sdks/invoices/README.md#postinvoicessearch) - List invoices by filter
- [`invoicesPutInvoicesId`](docs/sdks/invoices/README.md#putinvoicesid) - Update an invoice
- [`invoicesPutInvoicesIdPayment`](docs/sdks/invoices/README.md#putinvoicesidpayment) - Update invoice payment status
- [`paymentsDeletePaymentsId`](docs/sdks/payments/README.md#deletepaymentsid) - Delete a payment
- [`paymentsGetPayments`](docs/sdks/payments/README.md#getpayments) - List payments
- [`paymentsGetPaymentsId`](docs/sdks/payments/README.md#getpaymentsid) - Get a payment by ID
- [`paymentsPostPayments`](docs/sdks/payments/README.md#postpayments) - Create a new payment
- [`paymentsPostPaymentsIdProcess`](docs/sdks/payments/README.md#postpaymentsidprocess) - Process a payment
- [`paymentsPutPaymentsId`](docs/sdks/payments/README.md#putpaymentsid) - Update a payment
- [`plansDeletePlansId`](docs/sdks/plans/README.md#deleteplansid) - Delete a plan
- [`plansGetPlans`](docs/sdks/plans/README.md#getplans) - Get plans
- [`plansGetPlansId`](docs/sdks/plans/README.md#getplansid) - Get a plan
- [`plansPostPlans`](docs/sdks/plans/README.md#postplans) - Create a new plan
- [`plansPostPlansIdSyncSubscriptions`](docs/sdks/plans/README.md#postplansidsyncsubscriptions) - Synchronize plan prices
- [`plansPostPlansSearch`](docs/sdks/plans/README.md#postplanssearch) - List plans by filter
- [`plansPutPlansId`](docs/sdks/plans/README.md#putplansid) - Update a plan
- [`pricesDeletePricesId`](docs/sdks/prices/README.md#deletepricesid) - Delete a price
- [`pricesGetPrices`](docs/sdks/prices/README.md#getprices) - Get prices
- [`pricesGetPricesId`](docs/sdks/prices/README.md#getpricesid) - Get a price by ID
- [`pricesPostPrices`](docs/sdks/prices/README.md#postprices) - Create a new price
- [`pricesPostPricesBulk`](docs/sdks/prices/README.md#postpricesbulk) - Create multiple prices in bulk
- [`pricesPutPricesId`](docs/sdks/prices/README.md#putpricesid) - Update a price
- [`priceUnitsDeletePricesUnitsId`](docs/sdks/priceunits/README.md#deletepricesunitsid) - Delete a price unit
- [`priceUnitsGetPricesUnits`](docs/sdks/priceunits/README.md#getpricesunits) - List price units
- [`priceUnitsGetPricesUnitsCodeCode`](docs/sdks/priceunits/README.md#getpricesunitscodecode) - Get a price unit by code
- [`priceUnitsGetPricesUnitsId`](docs/sdks/priceunits/README.md#getpricesunitsid) - Get a price unit by ID
- [`priceUnitsPostPricesUnits`](docs/sdks/priceunits/README.md#postpricesunits) - Create a new price unit
- [`priceUnitsPostPricesUnitsSearch`](docs/sdks/priceunits/README.md#postpricesunitssearch) - List price units by filter
- [`priceUnitsPutPricesUnitsId`](docs/sdks/priceunits/README.md#putpricesunitsid) - Update a price unit
- [`rbacGetRBACRoles`](docs/sdks/rbac/README.md#getrbacroles) - List all RBAC roles
- [`rbacGetRBACRolesId`](docs/sdks/rbac/README.md#getrbacrolesid) - Get a specific RBAC role
- [`scheduledTasksDeleteTasksScheduledId`](docs/sdks/scheduledtasks/README.md#deletetasksscheduledid) - Delete a scheduled task
- [`scheduledTasksGetTasksScheduled`](docs/sdks/scheduledtasks/README.md#gettasksscheduled) - List scheduled tasks
- [`scheduledTasksGetTasksScheduledId`](docs/sdks/scheduledtasks/README.md#gettasksscheduledid) - Get a scheduled task
- [`scheduledTasksPostTasksScheduled`](docs/sdks/scheduledtasks/README.md#posttasksscheduled) - Create a scheduled task
- [`scheduledTasksPostTasksScheduledIdRun`](docs/sdks/scheduledtasks/README.md#posttasksscheduledidrun) - Trigger force run
- [`scheduledTasksPostTasksScheduledScheduleUpdateBillingPeriod`](docs/sdks/scheduledtasks/README.md#posttasksscheduledscheduleupdatebillingperiod) - Schedule update billing period
- [`scheduledTasksPutTasksScheduledId`](docs/sdks/scheduledtasks/README.md#puttasksscheduledid) - Update a scheduled task
- [`secretsDeleteSecretsApiKeysId`](docs/sdks/secrets/README.md#deletesecretsapikeysid) - Delete an API key
- [`secretsGetSecretsApiKeys`](docs/sdks/secrets/README.md#getsecretsapikeys) - List API keys
- [`secretsPostSecretsApiKeys`](docs/sdks/secrets/README.md#postsecretsapikeys) - Create a new API key
- [`subscriptionsDeleteSubscriptionsAddon`](docs/sdks/subscriptions/README.md#deletesubscriptionsaddon) - Remove addon from subscription
- [`subscriptionsDeleteSubscriptionsLineitemsId`](docs/sdks/subscriptions/README.md#deletesubscriptionslineitemsid) - Delete subscription line item
- [`subscriptionsGetSubscriptions`](docs/sdks/subscriptions/README.md#getsubscriptions) - List subscriptions
- [`subscriptionsGetSubscriptionsId`](docs/sdks/subscriptions/README.md#getsubscriptionsid) - Get subscription
- [`subscriptionsGetSubscriptionsIdAddonsAssociations`](docs/sdks/subscriptions/README.md#getsubscriptionsidaddonsassociations) - Get active addon associations
- [`subscriptionsGetSubscriptionsIdEntitlements`](docs/sdks/subscriptions/README.md#getsubscriptionsidentitlements) - Get subscription entitlements
- [`subscriptionsGetSubscriptionsIdGrantsUpcoming`](docs/sdks/subscriptions/README.md#getsubscriptionsidgrantsupcoming) - Get upcoming credit grant applications
- [`subscriptionsGetSubscriptionsIdPauses`](docs/sdks/subscriptions/README.md#getsubscriptionsidpauses) - List all pauses for a subscription
- [`subscriptionsPostSubscriptions`](docs/sdks/subscriptions/README.md#postsubscriptions) - Create subscription
- [`subscriptionsPostSubscriptionsAddon`](docs/sdks/subscriptions/README.md#postsubscriptionsaddon) - Add addon to subscription
- [`subscriptionsPostSubscriptionsIdActivate`](docs/sdks/subscriptions/README.md#postsubscriptionsidactivate) - Activate draft subscription
- [`subscriptionsPostSubscriptionsIdCancel`](docs/sdks/subscriptions/README.md#postsubscriptionsidcancel) - Cancel subscription
- [`subscriptionsPostSubscriptionsIdChangeExecute`](docs/sdks/subscriptions/README.md#postsubscriptionsidchangeexecute) - Execute subscription plan change
- [`subscriptionsPostSubscriptionsIdChangePreview`](docs/sdks/subscriptions/README.md#postsubscriptionsidchangepreview) - Preview subscription plan change
- [`subscriptionsPostSubscriptionsIdPause`](docs/sdks/subscriptions/README.md#postsubscriptionsidpause) - Pause a subscription
- [`subscriptionsPostSubscriptionsIdResume`](docs/sdks/subscriptions/README.md#postsubscriptionsidresume) - Resume a paused subscription
- [`subscriptionsPostSubscriptionsSearch`](docs/sdks/subscriptions/README.md#postsubscriptionssearch) - List subscriptions by filter
- [`subscriptionsPostSubscriptionsUsage`](docs/sdks/subscriptions/README.md#postsubscriptionsusage) - Get usage by subscription
- [`subscriptionsPutSubscriptionsLineitemsId`](docs/sdks/subscriptions/README.md#putsubscriptionslineitemsid) - Update subscription line item
- [`tasksGetTasks`](docs/sdks/tasks/README.md#gettasks) - List tasks
- [`tasksGetTasksId`](docs/sdks/tasks/README.md#gettasksid) - Get a task
- [`tasksGetTasksResult`](docs/sdks/tasks/README.md#gettasksresult) - Get task processing result
- [`tasksPostTasks`](docs/sdks/tasks/README.md#posttasks) - Create a new task
- [`tasksPutTasksIdStatus`](docs/sdks/tasks/README.md#puttasksidstatus) - Update task status
- [`taxAssociationsDeleteTaxesAssociationsId`](docs/sdks/taxassociations/README.md#deletetaxesassociationsid) - Delete tax association
- [`taxAssociationsGetTaxesAssociations`](docs/sdks/taxassociations/README.md#gettaxesassociations) - List tax associations
- [`taxAssociationsGetTaxesAssociationsId`](docs/sdks/taxassociations/README.md#gettaxesassociationsid) - Get Tax Association
- [`taxAssociationsPostTaxesAssociations`](docs/sdks/taxassociations/README.md#posttaxesassociations) - Create Tax Association
- [`taxAssociationsPutTaxesAssociationsId`](docs/sdks/taxassociations/README.md#puttaxesassociationsid) - Update tax association
- [`taxRatesDeleteTaxesRatesId`](docs/sdks/taxrates/README.md#deletetaxesratesid) - Delete a tax rate
- [`taxRatesGetTaxesRates`](docs/sdks/taxrates/README.md#gettaxesrates) - Get tax rates
- [`taxRatesGetTaxesRatesId`](docs/sdks/taxrates/README.md#gettaxesratesid) - Get a tax rate
- [`taxRatesPostTaxesRates`](docs/sdks/taxrates/README.md#posttaxesrates) - Create a tax rate
- [`taxRatesPutTaxesRatesId`](docs/sdks/taxrates/README.md#puttaxesratesid) - Update a tax rate
- [`tenantsGetTenantBilling`](docs/sdks/tenants/README.md#gettenantbilling) - Get billing usage for the current tenant
- [`tenantsGetTenantsId`](docs/sdks/tenants/README.md#gettenantsid) - Get tenant by ID
- [`tenantsPostTenants`](docs/sdks/tenants/README.md#posttenants) - Create a new tenant
- [`tenantsPutTenantsUpdate`](docs/sdks/tenants/README.md#puttenantsupdate) - Update a tenant
- [`usersGetUsersMe`](docs/sdks/users/README.md#getusersme) - Get user info
- [`usersPostUsers`](docs/sdks/users/README.md#postusers) - Create service account
- [`usersPostUsersSearch`](docs/sdks/users/README.md#postuserssearch) - List users with filters
- [`walletsGetCustomersIdWallets`](docs/sdks/wallets/README.md#getcustomersidwallets) - Get wallets by customer ID
- [`walletsGetCustomersWallets`](docs/sdks/wallets/README.md#getcustomerswallets) - Get Customer Wallets
- [`walletsGetWallets`](docs/sdks/wallets/README.md#getwallets) - List wallets
- [`walletsGetWalletsId`](docs/sdks/wallets/README.md#getwalletsid) - Get wallet by ID
- [`walletsGetWalletsIdBalanceRealTime`](docs/sdks/wallets/README.md#getwalletsidbalancerealtime) - Get wallet balance
- [`walletsGetWalletsIdTransactions`](docs/sdks/wallets/README.md#getwalletsidtransactions) - Get wallet transactions
- [`walletsPostWallets`](docs/sdks/wallets/README.md#postwallets) - Create a new wallet
- [`walletsPostWalletsIdTerminate`](docs/sdks/wallets/README.md#postwalletsidterminate) - Terminate a wallet
- [`walletsPostWalletsIdTopUp`](docs/sdks/wallets/README.md#postwalletsidtopup) - Top up wallet
- [`walletsPostWalletsSearch`](docs/sdks/wallets/README.md#postwalletssearch) - List wallets by filter
- [`walletsPostWalletsTransactionsSearch`](docs/sdks/wallets/README.md#postwalletstransactionssearch) - List wallet transactions by filter
- [`walletsPutWalletsId`](docs/sdks/wallets/README.md#putwalletsid) - Update a wallet
- [`webhooksPostWebhooksChargebeeTenantIdEnvironmentId`](docs/sdks/webhooks/README.md#postwebhookschargebeetenantidenvironmentid) - Handle Chargebee webhook events
- [`webhooksPostWebhooksHubspotTenantIdEnvironmentId`](docs/sdks/webhooks/README.md#postwebhookshubspottenantidenvironmentid) - Handle HubSpot webhook events
- [`webhooksPostWebhooksNomodTenantIdEnvironmentId`](docs/sdks/webhooks/README.md#postwebhooksnomodtenantidenvironmentid) - Handle Nomod webhook events
- [`webhooksPostWebhooksQuickbooksTenantIdEnvironmentId`](docs/sdks/webhooks/README.md#postwebhooksquickbookstenantidenvironmentid) - Handle QuickBooks webhook events
- [`webhooksPostWebhooksRazorpayTenantIdEnvironmentId`](docs/sdks/webhooks/README.md#postwebhooksrazorpaytenantidenvironmentid) - Handle Razorpay webhook events
- [`webhooksPostWebhooksStripeTenantIdEnvironmentId`](docs/sdks/webhooks/README.md#postwebhooksstripetenantidenvironmentid) - Handle Stripe webhook events

</details>
<!-- End Standalone functions [standalone-funcs] -->

<!-- Start File uploads [file-upload] -->
## File uploads

Certain SDK methods accept files as part of a multi-part request. It is possible and typically recommended to upload files as a stream rather than reading the entire contents into memory. This avoids excessive memory consumption and potentially crashing with out-of-memory errors when working with very large files. The following example demonstrates how to attach a file stream to a request.

> [!TIP]
>
> Depending on your JavaScript runtime, there are convenient utilities that return a handle to a file without reading the entire contents into memory:
>
> - **Node.js v20+:** Since v20, Node.js comes with a native `openAsBlob` function in [`node:fs`](https://nodejs.org/docs/latest-v20.x/api/fs.html#fsopenasblobpath-options).
> - **Bun:** The native [`Bun.file`](https://bun.sh/docs/api/file-io#reading-files-bun-file) function produces a file handle that can be used for streaming file uploads.
> - **Browsers:** All supported browsers return an instance to a [`File`](https://developer.mozilla.org/en-US/docs/Web/API/File) when reading the value from an `<input type="file">` element.
> - **Node.js v18:** A file stream can be created using the `fileFrom` helper from [`fetch-blob/from.js`](https://www.npmjs.com/package/fetch-blob).

```typescript
import { FlexPrice } from "flexprice-sdk-test";
import { openAsBlob } from "node:fs";

const flexPrice = new FlexPrice({
  serverURL: "https://api.example.com",
  apiKeyAuth: "<YOUR_API_KEY_HERE>",
});

async function run() {
  const result = await flexPrice.events.postEventsAnalytics(
    await openAsBlob("example.file"),
  );

  console.log(result);
}

run();

```
<!-- End File uploads [file-upload] -->

<!-- Start Retries [retries] -->
## Retries

Some of the endpoints in this SDK support retries.  If you use the SDK without any configuration, it will fall back to the default retry strategy provided by the API.  However, the default retry strategy can be overridden on a per-operation basis, or across the entire SDK.

To change the default retry strategy for a single API call, simply provide a retryConfig object to the call:
```typescript
import { FlexPrice } from "flexprice-sdk-test";

const flexPrice = new FlexPrice({
  serverURL: "https://api.example.com",
  apiKeyAuth: "<YOUR_API_KEY_HERE>",
});

async function run() {
  const result = await flexPrice.addons.getAddons({}, {
    retries: {
      strategy: "backoff",
      backoff: {
        initialInterval: 1,
        maxInterval: 50,
        exponent: 1.1,
        maxElapsedTime: 100,
      },
      retryConnectionErrors: false,
    },
  });

  console.log(result);
}

run();

```

If you'd like to override the default retry strategy for all operations that support retries, you can provide a retryConfig at SDK initialization:
```typescript
import { FlexPrice } from "flexprice-sdk-test";

const flexPrice = new FlexPrice({
  serverURL: "https://api.example.com",
  retryConfig: {
    strategy: "backoff",
    backoff: {
      initialInterval: 1,
      maxInterval: 50,
      exponent: 1.1,
      maxElapsedTime: 100,
    },
    retryConnectionErrors: false,
  },
  apiKeyAuth: "<YOUR_API_KEY_HERE>",
});

async function run() {
  const result = await flexPrice.addons.getAddons({});

  console.log(result);
}

run();

```
<!-- End Retries [retries] -->

<!-- Start Error Handling [errors] -->
## Error Handling

[`FlexPriceError`](./src/models/errors/flexpriceerror.ts) is the base class for all HTTP error responses. It has the following properties:

| Property            | Type       | Description                                                                             |
| ------------------- | ---------- | --------------------------------------------------------------------------------------- |
| `error.message`     | `string`   | Error message                                                                           |
| `error.statusCode`  | `number`   | HTTP response status code eg `404`                                                      |
| `error.headers`     | `Headers`  | HTTP response headers                                                                   |
| `error.body`        | `string`   | HTTP body. Can be empty string if no body is returned.                                  |
| `error.rawResponse` | `Response` | Raw HTTP response                                                                       |
| `error.data$`       |            | Optional. Some errors may contain structured data. [See Error Classes](#error-classes). |

### Example
```typescript
import { FlexPrice } from "flexprice-sdk-test";
import * as errors from "flexprice-sdk-test/models/errors";

const flexPrice = new FlexPrice({
  serverURL: "https://api.example.com",
  apiKeyAuth: "<YOUR_API_KEY_HERE>",
});

async function run() {
  try {
    const result = await flexPrice.addons.getAddons({});

    console.log(result);
  } catch (error) {
    // The base class for HTTP error responses
    if (error instanceof errors.FlexPriceError) {
      console.log(error.message);
      console.log(error.statusCode);
      console.log(error.body);
      console.log(error.headers);

      // Depending on the method different errors may be thrown
      if (error instanceof errors.ErrorsErrorResponse) {
        console.log(error.data$.error); // components.ErrorsErrorDetail
        console.log(error.data$.success); // boolean
      }
    }
  }
}

run();

```

### Error Classes
**Primary errors:**
* [`FlexPriceError`](./src/models/errors/flexpriceerror.ts): The base class for HTTP error responses.
  * [`ErrorsErrorResponse`](./src/models/errors/errorserrorresponse.ts): *

<details><summary>Less common errors (6)</summary>

<br />

**Network errors:**
* [`ConnectionError`](./src/models/errors/httpclienterrors.ts): HTTP client was unable to make a request to a server.
* [`RequestTimeoutError`](./src/models/errors/httpclienterrors.ts): HTTP request timed out due to an AbortSignal signal.
* [`RequestAbortedError`](./src/models/errors/httpclienterrors.ts): HTTP request was aborted by the client.
* [`InvalidRequestError`](./src/models/errors/httpclienterrors.ts): Any input used to create a request is invalid.
* [`UnexpectedClientError`](./src/models/errors/httpclienterrors.ts): Unrecognised or unexpected error.


**Inherit from [`FlexPriceError`](./src/models/errors/flexpriceerror.ts)**:
* [`ResponseValidationError`](./src/models/errors/responsevalidationerror.ts): Type mismatch between the data returned from the server and the structure expected by the SDK. See `error.rawValue` for the raw value and `error.pretty()` for a nicely formatted multi-line string.

</details>

\* Check [the method documentation](#available-resources-and-operations) to see if the error is applicable.
<!-- End Error Handling [errors] -->

<!-- Start Custom HTTP Client [http-client] -->
## Custom HTTP Client

The TypeScript SDK makes API calls using an `HTTPClient` that wraps the native
[Fetch API](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API). This
client is a thin wrapper around `fetch` and provides the ability to attach hooks
around the request lifecycle that can be used to modify the request or handle
errors and response.

The `HTTPClient` constructor takes an optional `fetcher` argument that can be
used to integrate a third-party HTTP client or when writing tests to mock out
the HTTP client and feed in fixtures.

The following example shows how to use the `"beforeRequest"` hook to to add a
custom header and a timeout to requests and how to use the `"requestError"` hook
to log errors:

```typescript
import { FlexPrice } from "flexprice-sdk-test";
import { HTTPClient } from "flexprice-sdk-test/lib/http";

const httpClient = new HTTPClient({
  // fetcher takes a function that has the same signature as native `fetch`.
  fetcher: (request) => {
    return fetch(request);
  }
});

httpClient.addHook("beforeRequest", (request) => {
  const nextRequest = new Request(request, {
    signal: request.signal || AbortSignal.timeout(5000)
  });

  nextRequest.headers.set("x-custom-header", "custom value");

  return nextRequest;
});

httpClient.addHook("requestError", (error, request) => {
  console.group("Request Error");
  console.log("Reason:", `${error}`);
  console.log("Endpoint:", `${request.method} ${request.url}`);
  console.groupEnd();
});

const sdk = new FlexPrice({ httpClient: httpClient });
```
<!-- End Custom HTTP Client [http-client] -->

<!-- Start Debugging [debug] -->
## Debugging

You can setup your SDK to emit debug logs for SDK requests and responses.

You can pass a logger that matches `console`'s interface as an SDK option.

> [!WARNING]
> Beware that debug logging will reveal secrets, like API tokens in headers, in log messages printed to a console or files. It's recommended to use this feature only during local development and not in production.

```typescript
import { FlexPrice } from "flexprice-sdk-test";

const sdk = new FlexPrice({ debugLogger: console });
```
<!-- End Debugging [debug] -->

<!-- Placeholder for Future Speakeasy SDK Sections -->
