# FlexPrice Python SDK

This is the Python client library for the FlexPrice API.

## Installation

```bash
pip install flexprice
```

## Usage

```python
"""
FlexPrice Python SDK Example

This example demonstrates how to use the FlexPrice Python SDK
to interact with the FlexPrice API.
"""

import os
import time
import datetime
from pprint import pprint

# Import the FlexPrice SDK
import flexprice
from flexprice.api import customers_api, events_api
from flexprice.models.dto_create_customer_request import DtoCreateCustomerRequest
from flexprice.models.dto_ingest_event_request import DtoIngestEventRequest

# Optional: Load environment variables from .env file
from dotenv import load_dotenv
load_dotenv()


def run_example():
    """Main example function demonstrating FlexPrice SDK usage."""
    print("Starting FlexPrice Python SDK example...")

    try:
        # Configure the API client
        api_key = os.getenv("FLEXPRICE_API_KEY")
        api_host = os.getenv("FLEXPRICE_API_HOST", "api.cloud.flexprice.io")

        if not api_key:
            raise ValueError("FLEXPRICE_API_KEY environment variable is required")
            
        print("Using API Key:", api_key[:4] + "..." + api_key[-4:])  # Show just the start and end for security

        # Configure API key authorization
        configuration = flexprice.Configuration(
            host=f"https://{api_host}/v1"
        )
        configuration.api_key['x-api-key'] = api_key
       
        # Create API client
        with flexprice.ApiClient(configuration) as api_client:
            # Set the API key header
            api_client.default_headers['x-api-key'] = api_key
            # Add User-Agent header
            configuration.user_agent = "FlexPricePythonSDK/1.0.0 Example"
            # Print actual headers for debugging
            
            # Create API instances
            events_api_instance = events_api.EventsApi(api_client)

            # Generate a unique customer ID for this example
            customer_id = f"sample-customer-{int(time.time())}"
            
            print(f"Creating customer with ID: {customer_id}...")

            # Step 1: Create an event
            print("Creating event...")
            
            event_request = DtoIngestEventRequest(
                event_name="Sample Event",
                external_customer_id=customer_id,
                properties={
                    "source": "python_sample_app",
                    "environment": "test",
                    "timestamp": datetime.datetime.now().isoformat()
                },
                source="python_sample_app"
            )
            
            event_result = events_api_instance.events_post(event=event_request)
            print(f"Event created successfully! ID: {event_result.event_id if hasattr(event_result, 'event_id') else 'unknown'}")

            # Step 2: Retrieve events for this customer
            print(f"Retrieving events for customer {customer_id}...")
            
            events_response = events_api_instance.events_get(external_customer_id=customer_id)
            
            # Check if events are available in the response
            if hasattr(events_response, 'events') and events_response.events:
                print(f"Found {len(events_response.events)} events:")
                
                for i, event in enumerate(events_response.events):
                    print(f"Event {i+1}: {event.id if hasattr(event, 'id') else 'unknown'} - {event.event_name if hasattr(event, 'event_name') else 'unknown'}")
                    print(f"Properties: {event.properties if hasattr(event, 'properties') else {}}")
            else:
                print("No events found or events not available in response.")
            
            print("Example completed successfully!")

    except flexprice.ApiException as e:
        print(f"\n=== API Exception ===")
        print(f"Status code: {e.status}")
        print(f"Reason: {e.reason}")
        print(f"HTTP response headers: {e.headers}")
        print(f"HTTP response body: {e.body}")    
    except ValueError as e:
        print(f"Value error: {e}")
    except Exception as e:
        print(f"Unexpected error: {e}")
```

## Asynchronous Event Submission

The FlexPrice SDK provides asynchronous event submission functionality that allows you to:

- Submit events in a non-blocking manner with "fire-and-forget" capability
- Include optional callbacks to handle success/failure responses
- Automatically retry failed event submissions with exponential backoff
- Process events in background threads

### Basic Async Usage

```python
from flexprice import Configuration, ApiClient, EventsApi
from flexprice.models import DtoIngestEventRequest

# Configure the client
configuration = Configuration(api_key={'ApiKeyAuth': 'YOUR_API_KEY'})
configuration.host = "https://api.cloud.flexprice.io/v1"

# Create API client and event API instance
api_client = ApiClient(configuration)
events_api = EventsApi(api_client)

# Create an event
event = DtoIngestEventRequest(
    external_customer_id="customer123",
    event_name="api_call",
    properties={"region": "us-west", "method": "GET"},
    source="my_application"
)

# Submit asynchronously (fire-and-forget)
events_api.events_post_async(event)
```

### Using Callbacks

```python
# Define a callback function
def on_event_processed(result, error, success):
    if success:
        print(f"Event processed successfully: {result}")
    else:
        print(f"Event processing failed: {error}")

# Create and submit event with callback
event = DtoIngestEventRequest(
    external_customer_id="customer123",
    event_name="user_action",
    properties={"action": "login", "device": "mobile"},
    source="user_portal"
)

# Submit with callback
events_api.events_post_async(event, callback=on_event_processed)
```

### Complete Example

For a complete example of asynchronous event submission, see the `async_event_example.py` file in the examples directory.

## Running the Example

To run the provided example:

1. Clone the repository:
   ```bash
   git clone https://github.com/flexprice/python-sdk.git
   cd python-sdk/examples
   ```

2. Create a virtual environment and install dependencies:
   ```bash
   python -m venv venv
   source venv/bin/activate  # On Windows: venv\Scripts\activate
   pip install -r requirements.txt
   ```

3. Create a `.env` file with your API credentials:
   ```bash
   cp .env.sample .env
   # Edit .env with your API key
   ```

4. Run the example:
   ```bash
   python example.py
   ```

5. Run the async example:
   ```bash
   python async_event_example.py
   ```

## Features

- Complete API coverage
- Strong type hints
- Detailed documentation
- Error handling
- Asynchronous support for event submission

## Documentation

For detailed API documentation, refer to the code comments and the official FlexPrice API documentation.

## Advanced Usage

### Handling Errors

The SDK provides detailed error information through exceptions:

```python
try:
    # API call
    result = client.some_api_call()
except flexprice.ApiException as e:
    print(f"API exception: {e}")
    print(f"Status code: {e.status}")
    print(f"Response body: {e.body}")
except Exception as e:
    print(f"General exception: {e}")
```

### Asynchronous API Usage with asyncio

In addition to the built-in asynchronous event submission, the SDK can be used with libraries like `asyncio` for other operations:

```python
import asyncio
import flexprice
from flexprice.api import customers_api

async def get_customer(customer_id):
    configuration = flexprice.Configuration(
        host="https://api.cloud.flexprice.io/v1"
    )
    configuration.api_key['x-api-key'] = "your-api-key"
    
    async with flexprice.ApiClient(configuration) as api_client:
        api = customers_api.CustomersApi(api_client)
        return await api.customers_id_get(id=customer_id)

# Run with asyncio
customer = asyncio.run(get_customer("customer-123"))
print(customer)
``` 
<!-- Start Summary [summary] -->
## Summary

FlexPrice API: FlexPrice API Service
<!-- End Summary [summary] -->

<!-- Start Table of Contents [toc] -->
## Table of Contents
<!-- $toc-max-depth=2 -->
* [FlexPrice Python SDK](#flexprice-python-sdk)
  * [Installation](#installation)
  * [Usage](#usage)
  * [Asynchronous Event Submission](#asynchronous-event-submission)
  * [Running the Example](#running-the-example)
* [Run with asyncio](#run-with-asyncio)
  * [SDK Installation](#sdk-installation)
  * [IDE Support](#ide-support)
  * [SDK Example Usage](#sdk-example-usage)
  * [Authentication](#authentication)
  * [Available Resources and Operations](#available-resources-and-operations)
  * [File uploads](#file-uploads)
  * [Retries](#retries)
  * [Error Handling](#error-handling)
  * [Custom HTTP Client](#custom-http-client)
  * [Resource Management](#resource-management)
  * [Debugging](#debugging)

<!-- End Table of Contents [toc] -->

<!-- Start SDK Installation [installation] -->
## SDK Installation

> [!NOTE]
> **Python version upgrade policy**
>
> Once a Python version reaches its [official end of life date](https://devguide.python.org/versions/), a 3-month grace period is provided for users to upgrade. Following this grace period, the minimum python version supported in the SDK will be updated.

The SDK can be installed with *uv*, *pip*, or *poetry* package managers.

### uv

*uv* is a fast Python package installer and resolver, designed as a drop-in replacement for pip and pip-tools. It's recommended for its speed and modern Python tooling capabilities.

```bash
uv add flexprice-sdk-test
```

### PIP

*PIP* is the default package installer for Python, enabling easy installation and management of packages from PyPI via the command line.

```bash
pip install flexprice-sdk-test
```

### Poetry

*Poetry* is a modern tool that simplifies dependency management and package publishing by using a single `pyproject.toml` file to handle project metadata and dependencies.

```bash
poetry add flexprice-sdk-test
```

### Shell and script usage with `uv`

You can use this SDK in a Python shell with [uv](https://docs.astral.sh/uv/) and the `uvx` command that comes with it like so:

```shell
uvx --from flexprice-sdk-test python
```

It's also possible to write a standalone Python script without needing to set up a whole project like so:

```python
#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.9"
# dependencies = [
#     "flexprice-sdk-test",
# ]
# ///

from flexprice_sdk_test import FlexPrice

sdk = FlexPrice(
  # SDK arguments
)

# Rest of script here...
```

Once that is saved to a file, you can run it with `uv run script.py` where
`script.py` can be replaced with the actual file name.
<!-- End SDK Installation [installation] -->

<!-- Start IDE Support [idesupport] -->
## IDE Support

### PyCharm

Generally, the SDK will work well with most IDEs out of the box. However, when using PyCharm, you can enjoy much better integration with Pydantic by installing an additional plugin.

- [PyCharm Pydantic Plugin](https://docs.pydantic.dev/latest/integrations/pycharm/)
<!-- End IDE Support [idesupport] -->

<!-- Start SDK Example Usage [usage] -->
## SDK Example Usage

### Example

```python
# Synchronous Example
from flexprice_sdk_test import FlexPrice


with FlexPrice(
    server_url="https://api.example.com",
    api_key_auth="<YOUR_API_KEY_HERE>",
) as flex_price:

    res = flex_price.addons.get_addons()

    # Handle response
    print(res)
```

</br>

The same SDK client can also be used to make asynchronous requests by importing asyncio.

```python
# Asynchronous Example
import asyncio
from flexprice_sdk_test import FlexPrice

async def main():

    async with FlexPrice(
        server_url="https://api.example.com",
        api_key_auth="<YOUR_API_KEY_HERE>",
    ) as flex_price:

        res = await flex_price.addons.get_addons_async()

        # Handle response
        print(res)

asyncio.run(main())
```
<!-- End SDK Example Usage [usage] -->

<!-- Start Authentication [security] -->
## Authentication

### Per-Client Security Schemes

This SDK supports the following security scheme globally:

| Name           | Type   | Scheme  |
| -------------- | ------ | ------- |
| `api_key_auth` | apiKey | API key |

To authenticate with the API the `api_key_auth` parameter must be set when initializing the SDK client instance. For example:
```python
from flexprice_sdk_test import FlexPrice


with FlexPrice(
    server_url="https://api.example.com",
    api_key_auth="<YOUR_API_KEY_HERE>",
) as flex_price:

    res = flex_price.addons.get_addons()

    # Handle response
    print(res)

```
<!-- End Authentication [security] -->

<!-- Start Available Resources and Operations [operations] -->
## Available Resources and Operations

<details open>
<summary>Available methods</summary>

### [Addons](docs/sdks/addons/README.md)

* [get_addons](docs/sdks/addons/README.md#get_addons) - List addons
* [post_addons](docs/sdks/addons/README.md#post_addons) - Create addon
* [get_addons_lookup_lookup_key_](docs/sdks/addons/README.md#get_addons_lookup_lookup_key_) - Get addon by lookup key
* [post_addons_search](docs/sdks/addons/README.md#post_addons_search) - List addons by filter
* [get_addons_id_](docs/sdks/addons/README.md#get_addons_id_) - Get addon
* [put_addons_id_](docs/sdks/addons/README.md#put_addons_id_) - Update addon
* [delete_addons_id_](docs/sdks/addons/README.md#delete_addons_id_) - Delete addon

### [AlertLogs](docs/sdks/alertlogs/README.md)

* [post_alert_search](docs/sdks/alertlogs/README.md#post_alert_search) - List alert logs by filter

### [Auth](docs/sdks/auth/README.md)

* [post_auth_login](docs/sdks/auth/README.md#post_auth_login) - Login
* [post_auth_signup](docs/sdks/auth/README.md#post_auth_signup) - Sign up

### [Connections](docs/sdks/connections/README.md)

* [get_connections](docs/sdks/connections/README.md#get_connections) - Get connections
* [post_connections_search](docs/sdks/connections/README.md#post_connections_search) - List connections by filter
* [get_connections_id_](docs/sdks/connections/README.md#get_connections_id_) - Get a connection
* [put_connections_id_](docs/sdks/connections/README.md#put_connections_id_) - Update a connection
* [delete_connections_id_](docs/sdks/connections/README.md#delete_connections_id_) - Delete a connection

### [Costs](docs/sdks/costs/README.md)

* [post_costs](docs/sdks/costs/README.md#post_costs) - Create a new costsheet
* [get_costs_active](docs/sdks/costs/README.md#get_costs_active) - Get active costsheet for tenant
* [post_costs_analytics](docs/sdks/costs/README.md#post_costs_analytics) - Get combined revenue and cost analytics
* [post_costs_analytics_v2](docs/sdks/costs/README.md#post_costs_analytics_v2) - Get combined revenue and cost analytics
* [post_costs_search](docs/sdks/costs/README.md#post_costs_search) - List costsheets by filter
* [get_costs_id_](docs/sdks/costs/README.md#get_costs_id_) - Get a costsheet by ID
* [put_costs_id_](docs/sdks/costs/README.md#put_costs_id_) - Update a costsheet
* [delete_costs_id_](docs/sdks/costs/README.md#delete_costs_id_) - Delete a costsheet

### [Coupons](docs/sdks/coupons/README.md)

* [get_coupons](docs/sdks/coupons/README.md#get_coupons) - List coupons with filtering
* [post_coupons](docs/sdks/coupons/README.md#post_coupons) - Create a new coupon
* [get_coupons_id_](docs/sdks/coupons/README.md#get_coupons_id_) - Get a coupon by ID
* [put_coupons_id_](docs/sdks/coupons/README.md#put_coupons_id_) - Update a coupon
* [delete_coupons_id_](docs/sdks/coupons/README.md#delete_coupons_id_) - Delete a coupon

### [CreditNotes](docs/sdks/creditnotes/README.md)

* [get_creditnotes](docs/sdks/creditnotes/README.md#get_creditnotes) - List credit notes with filtering
* [post_creditnotes](docs/sdks/creditnotes/README.md#post_creditnotes) - Create a new credit note
* [get_creditnotes_id_](docs/sdks/creditnotes/README.md#get_creditnotes_id_) - Get a credit note by ID
* [post_creditnotes_id_finalize](docs/sdks/creditnotes/README.md#post_creditnotes_id_finalize) - Process a draft credit note
* [post_creditnotes_id_void](docs/sdks/creditnotes/README.md#post_creditnotes_id_void) - Void a credit note

### [CreditGrants](docs/sdks/creditgrants/README.md)

* [get_creditgrants](docs/sdks/creditgrants/README.md#get_creditgrants) - Get credit grants
* [post_creditgrants](docs/sdks/creditgrants/README.md#post_creditgrants) - Create a new credit grant
* [get_creditgrants_id_](docs/sdks/creditgrants/README.md#get_creditgrants_id_) - Get a credit grant by ID
* [put_creditgrants_id_](docs/sdks/creditgrants/README.md#put_creditgrants_id_) - Update a credit grant
* [delete_creditgrants_id_](docs/sdks/creditgrants/README.md#delete_creditgrants_id_) - Delete a credit grant
* [get_plans_id_creditgrants](docs/sdks/creditgrants/README.md#get_plans_id_creditgrants) - Get plan credit grants

### [Customers](docs/sdks/customers/README.md)

* [get_customers](docs/sdks/customers/README.md#get_customers) - Get customers
* [post_customers](docs/sdks/customers/README.md#post_customers) - Create a customer
* [get_customers_external_external_id_](docs/sdks/customers/README.md#get_customers_external_external_id_) - Get a customer by external id
* [post_customers_search](docs/sdks/customers/README.md#post_customers_search) - List customers by filter
* [get_customers_usage](docs/sdks/customers/README.md#get_customers_usage) - Get customer usage summary
* [get_customers_id_](docs/sdks/customers/README.md#get_customers_id_) - Get a customer
* [put_customers_id_](docs/sdks/customers/README.md#put_customers_id_) - Update a customer
* [delete_customers_id_](docs/sdks/customers/README.md#delete_customers_id_) - Delete a customer
* [get_customers_id_entitlements](docs/sdks/customers/README.md#get_customers_id_entitlements) - Get customer entitlements
* [get_customers_id_grants_upcoming](docs/sdks/customers/README.md#get_customers_id_grants_upcoming) - Get upcoming credit grant applications

### [Entitlements](docs/sdks/entitlements/README.md)

* [get_addons_id_entitlements](docs/sdks/entitlements/README.md#get_addons_id_entitlements) - Get addon entitlements
* [get_entitlements](docs/sdks/entitlements/README.md#get_entitlements) - Get entitlements
* [post_entitlements](docs/sdks/entitlements/README.md#post_entitlements) - Create a new entitlement
* [post_entitlements_bulk](docs/sdks/entitlements/README.md#post_entitlements_bulk) - Create multiple entitlements in bulk
* [post_entitlements_search](docs/sdks/entitlements/README.md#post_entitlements_search) - List entitlements by filter
* [get_entitlements_id_](docs/sdks/entitlements/README.md#get_entitlements_id_) - Get an entitlement by ID
* [put_entitlements_id_](docs/sdks/entitlements/README.md#put_entitlements_id_) - Update an entitlement
* [delete_entitlements_id_](docs/sdks/entitlements/README.md#delete_entitlements_id_) - Delete an entitlement
* [get_plans_id_entitlements](docs/sdks/entitlements/README.md#get_plans_id_entitlements) - Get plan entitlements

### [EntityIntegrationMappings](docs/sdks/entityintegrationmappings/README.md)

* [get_entity_integration_mappings](docs/sdks/entityintegrationmappings/README.md#get_entity_integration_mappings) - List entity integration mappings
* [post_entity_integration_mappings](docs/sdks/entityintegrationmappings/README.md#post_entity_integration_mappings) - Create entity integration mapping
* [get_entity_integration_mappings_id_](docs/sdks/entityintegrationmappings/README.md#get_entity_integration_mappings_id_) - Get entity integration mapping
* [delete_entity_integration_mappings_id_](docs/sdks/entityintegrationmappings/README.md#delete_entity_integration_mappings_id_) - Delete entity integration mapping

### [Environments](docs/sdks/environments/README.md)

* [get_environments](docs/sdks/environments/README.md#get_environments) - Get environments
* [post_environments](docs/sdks/environments/README.md#post_environments) - Create an environment
* [get_environments_id_](docs/sdks/environments/README.md#get_environments_id_) - Get an environment
* [put_environments_id_](docs/sdks/environments/README.md#put_environments_id_) - Update an environment

### [Events](docs/sdks/events/README.md)

* [post_events](docs/sdks/events/README.md#post_events) - Ingest event
* [post_events_analytics](docs/sdks/events/README.md#post_events_analytics) - Get usage analytics
* [post_events_bulk](docs/sdks/events/README.md#post_events_bulk) - Bulk Ingest events
* [post_events_huggingface_inference](docs/sdks/events/README.md#post_events_huggingface_inference) - Get hugging face inference data
* [get_events_monitoring](docs/sdks/events/README.md#get_events_monitoring) - Get monitoring data
* [post_events_query](docs/sdks/events/README.md#post_events_query) - List raw events
* [post_events_usage](docs/sdks/events/README.md#post_events_usage) - Get usage statistics
* [post_events_usage_meter](docs/sdks/events/README.md#post_events_usage_meter) - Get usage by meter

### [Features](docs/sdks/features/README.md)

* [get_features](docs/sdks/features/README.md#get_features) - List features
* [post_features](docs/sdks/features/README.md#post_features) - Create a new feature
* [post_features_search](docs/sdks/features/README.md#post_features_search) - List features by filter
* [get_features_id_](docs/sdks/features/README.md#get_features_id_) - Get a feature by ID
* [put_features_id_](docs/sdks/features/README.md#put_features_id_) - Update a feature
* [delete_features_id_](docs/sdks/features/README.md#delete_features_id_) - Delete a feature

### [Groups](docs/sdks/groups/README.md)

* [post_groups](docs/sdks/groups/README.md#post_groups) - Create a group
* [post_groups_search](docs/sdks/groups/README.md#post_groups_search) - Get groups
* [get_groups_id_](docs/sdks/groups/README.md#get_groups_id_) - Get a group
* [delete_groups_id_](docs/sdks/groups/README.md#delete_groups_id_) - Delete a group

### [Integrations](docs/sdks/integrations/README.md)

* [get_secrets_integrations_by_provider_provider_](docs/sdks/integrations/README.md#get_secrets_integrations_by_provider_provider_) - Get integration details
* [post_secrets_integrations_create_provider_](docs/sdks/integrations/README.md#post_secrets_integrations_create_provider_) - Create or update an integration
* [get_secrets_integrations_linked](docs/sdks/integrations/README.md#get_secrets_integrations_linked) - List linked integrations
* [delete_secrets_integrations_id_](docs/sdks/integrations/README.md#delete_secrets_integrations_id_) - Delete an integration

### [Invoices](docs/sdks/invoices/README.md)

* [get_customers_id_invoices_summary](docs/sdks/invoices/README.md#get_customers_id_invoices_summary) - Get a customer invoice summary
* [get_invoices](docs/sdks/invoices/README.md#get_invoices) - List invoices
* [post_invoices](docs/sdks/invoices/README.md#post_invoices) - Create a new one off invoice
* [post_invoices_preview](docs/sdks/invoices/README.md#post_invoices_preview) - Get a preview invoice
* [post_invoices_search](docs/sdks/invoices/README.md#post_invoices_search) - List invoices by filter
* [get_invoices_id_](docs/sdks/invoices/README.md#get_invoices_id_) - Get an invoice by ID
* [put_invoices_id_](docs/sdks/invoices/README.md#put_invoices_id_) - Update an invoice
* [post_invoices_id_comms_trigger](docs/sdks/invoices/README.md#post_invoices_id_comms_trigger) - Trigger communication webhook for an invoice
* [post_invoices_id_finalize](docs/sdks/invoices/README.md#post_invoices_id_finalize) - Finalize an invoice
* [put_invoices_id_payment](docs/sdks/invoices/README.md#put_invoices_id_payment) - Update invoice payment status
* [post_invoices_id_payment_attempt](docs/sdks/invoices/README.md#post_invoices_id_payment_attempt) - Attempt payment for an invoice
* [get_invoices_id_pdf](docs/sdks/invoices/README.md#get_invoices_id_pdf) - Get PDF for an invoice
* [post_invoices_id_recalculate](docs/sdks/invoices/README.md#post_invoices_id_recalculate) - Recalculate invoice totals and line items
* [post_invoices_id_void](docs/sdks/invoices/README.md#post_invoices_id_void) - Void an invoice

### [Payments](docs/sdks/payments/README.md)

* [get_payments](docs/sdks/payments/README.md#get_payments) - List payments
* [post_payments](docs/sdks/payments/README.md#post_payments) - Create a new payment
* [get_payments_id_](docs/sdks/payments/README.md#get_payments_id_) - Get a payment by ID
* [put_payments_id_](docs/sdks/payments/README.md#put_payments_id_) - Update a payment
* [delete_payments_id_](docs/sdks/payments/README.md#delete_payments_id_) - Delete a payment
* [post_payments_id_process](docs/sdks/payments/README.md#post_payments_id_process) - Process a payment

### [Plans](docs/sdks/plans/README.md)

* [get_plans](docs/sdks/plans/README.md#get_plans) - Get plans
* [post_plans](docs/sdks/plans/README.md#post_plans) - Create a new plan
* [post_plans_search](docs/sdks/plans/README.md#post_plans_search) - List plans by filter
* [get_plans_id_](docs/sdks/plans/README.md#get_plans_id_) - Get a plan
* [put_plans_id_](docs/sdks/plans/README.md#put_plans_id_) - Update a plan
* [delete_plans_id_](docs/sdks/plans/README.md#delete_plans_id_) - Delete a plan
* [post_plans_id_sync_subscriptions](docs/sdks/plans/README.md#post_plans_id_sync_subscriptions) - Synchronize plan prices

### [PriceUnits](docs/sdks/priceunits/README.md)

* [get_prices_units](docs/sdks/priceunits/README.md#get_prices_units) - List price units
* [post_prices_units](docs/sdks/priceunits/README.md#post_prices_units) - Create a new price unit
* [get_prices_units_code_code_](docs/sdks/priceunits/README.md#get_prices_units_code_code_) - Get a price unit by code
* [post_prices_units_search](docs/sdks/priceunits/README.md#post_prices_units_search) - List price units by filter
* [get_prices_units_id_](docs/sdks/priceunits/README.md#get_prices_units_id_) - Get a price unit by ID
* [put_prices_units_id_](docs/sdks/priceunits/README.md#put_prices_units_id_) - Update a price unit
* [delete_prices_units_id_](docs/sdks/priceunits/README.md#delete_prices_units_id_) - Delete a price unit

### [Prices](docs/sdks/prices/README.md)

* [get_prices](docs/sdks/prices/README.md#get_prices) - Get prices
* [post_prices](docs/sdks/prices/README.md#post_prices) - Create a new price
* [post_prices_bulk](docs/sdks/prices/README.md#post_prices_bulk) - Create multiple prices in bulk
* [get_prices_id_](docs/sdks/prices/README.md#get_prices_id_) - Get a price by ID
* [put_prices_id_](docs/sdks/prices/README.md#put_prices_id_) - Update a price
* [delete_prices_id_](docs/sdks/prices/README.md#delete_prices_id_) - Delete a price

### [Rbac](docs/sdks/rbac/README.md)

* [get_rbac_roles](docs/sdks/rbac/README.md#get_rbac_roles) - List all RBAC roles
* [get_rbac_roles_id_](docs/sdks/rbac/README.md#get_rbac_roles_id_) - Get a specific RBAC role

### [ScheduledTasks](docs/sdks/scheduledtasks/README.md)

* [get_tasks_scheduled](docs/sdks/scheduledtasks/README.md#get_tasks_scheduled) - List scheduled tasks
* [post_tasks_scheduled](docs/sdks/scheduledtasks/README.md#post_tasks_scheduled) - Create a scheduled task
* [post_tasks_scheduled_schedule_update_billing_period](docs/sdks/scheduledtasks/README.md#post_tasks_scheduled_schedule_update_billing_period) - Schedule update billing period
* [get_tasks_scheduled_id_](docs/sdks/scheduledtasks/README.md#get_tasks_scheduled_id_) - Get a scheduled task
* [put_tasks_scheduled_id_](docs/sdks/scheduledtasks/README.md#put_tasks_scheduled_id_) - Update a scheduled task
* [delete_tasks_scheduled_id_](docs/sdks/scheduledtasks/README.md#delete_tasks_scheduled_id_) - Delete a scheduled task
* [post_tasks_scheduled_id_run](docs/sdks/scheduledtasks/README.md#post_tasks_scheduled_id_run) - Trigger force run

### [Secrets](docs/sdks/secrets/README.md)

* [get_secrets_api_keys](docs/sdks/secrets/README.md#get_secrets_api_keys) - List API keys
* [post_secrets_api_keys](docs/sdks/secrets/README.md#post_secrets_api_keys) - Create a new API key
* [delete_secrets_api_keys_id_](docs/sdks/secrets/README.md#delete_secrets_api_keys_id_) - Delete an API key

### [Subscriptions](docs/sdks/subscriptions/README.md)

* [get_subscriptions](docs/sdks/subscriptions/README.md#get_subscriptions) - List subscriptions
* [post_subscriptions](docs/sdks/subscriptions/README.md#post_subscriptions) - Create subscription
* [post_subscriptions_addon](docs/sdks/subscriptions/README.md#post_subscriptions_addon) - Add addon to subscription
* [delete_subscriptions_addon](docs/sdks/subscriptions/README.md#delete_subscriptions_addon) - Remove addon from subscription
* [put_subscriptions_lineitems_id_](docs/sdks/subscriptions/README.md#put_subscriptions_lineitems_id_) - Update subscription line item
* [delete_subscriptions_lineitems_id_](docs/sdks/subscriptions/README.md#delete_subscriptions_lineitems_id_) - Delete subscription line item
* [post_subscriptions_search](docs/sdks/subscriptions/README.md#post_subscriptions_search) - List subscriptions by filter
* [post_subscriptions_usage](docs/sdks/subscriptions/README.md#post_subscriptions_usage) - Get usage by subscription
* [get_subscriptions_id_](docs/sdks/subscriptions/README.md#get_subscriptions_id_) - Get subscription
* [post_subscriptions_id_activate](docs/sdks/subscriptions/README.md#post_subscriptions_id_activate) - Activate draft subscription
* [get_subscriptions_id_addons_associations](docs/sdks/subscriptions/README.md#get_subscriptions_id_addons_associations) - Get active addon associations
* [post_subscriptions_id_cancel](docs/sdks/subscriptions/README.md#post_subscriptions_id_cancel) - Cancel subscription
* [post_subscriptions_id_change_execute](docs/sdks/subscriptions/README.md#post_subscriptions_id_change_execute) - Execute subscription plan change
* [post_subscriptions_id_change_preview](docs/sdks/subscriptions/README.md#post_subscriptions_id_change_preview) - Preview subscription plan change
* [get_subscriptions_id_entitlements](docs/sdks/subscriptions/README.md#get_subscriptions_id_entitlements) - Get subscription entitlements
* [get_subscriptions_id_grants_upcoming](docs/sdks/subscriptions/README.md#get_subscriptions_id_grants_upcoming) - Get upcoming credit grant applications
* [post_subscriptions_id_pause](docs/sdks/subscriptions/README.md#post_subscriptions_id_pause) - Pause a subscription
* [get_subscriptions_id_pauses](docs/sdks/subscriptions/README.md#get_subscriptions_id_pauses) - List all pauses for a subscription
* [post_subscriptions_id_resume](docs/sdks/subscriptions/README.md#post_subscriptions_id_resume) - Resume a paused subscription

### [Tasks](docs/sdks/tasks/README.md)

* [get_tasks](docs/sdks/tasks/README.md#get_tasks) - List tasks
* [post_tasks](docs/sdks/tasks/README.md#post_tasks) - Create a new task
* [get_tasks_result](docs/sdks/tasks/README.md#get_tasks_result) - Get task processing result
* [get_tasks_id_](docs/sdks/tasks/README.md#get_tasks_id_) - Get a task
* [put_tasks_id_status](docs/sdks/tasks/README.md#put_tasks_id_status) - Update task status

### [TaxAssociations](docs/sdks/taxassociations/README.md)

* [get_taxes_associations](docs/sdks/taxassociations/README.md#get_taxes_associations) - List tax associations
* [post_taxes_associations](docs/sdks/taxassociations/README.md#post_taxes_associations) - Create Tax Association
* [get_taxes_associations_id_](docs/sdks/taxassociations/README.md#get_taxes_associations_id_) - Get Tax Association
* [put_taxes_associations_id_](docs/sdks/taxassociations/README.md#put_taxes_associations_id_) - Update tax association
* [delete_taxes_associations_id_](docs/sdks/taxassociations/README.md#delete_taxes_associations_id_) - Delete tax association

### [TaxRates](docs/sdks/taxrates/README.md)

* [get_taxes_rates](docs/sdks/taxrates/README.md#get_taxes_rates) - Get tax rates
* [post_taxes_rates](docs/sdks/taxrates/README.md#post_taxes_rates) - Create a tax rate
* [get_taxes_rates_id_](docs/sdks/taxrates/README.md#get_taxes_rates_id_) - Get a tax rate
* [put_taxes_rates_id_](docs/sdks/taxrates/README.md#put_taxes_rates_id_) - Update a tax rate
* [delete_taxes_rates_id_](docs/sdks/taxrates/README.md#delete_taxes_rates_id_) - Delete a tax rate

### [Tenants](docs/sdks/tenants/README.md)

* [get_tenant_billing](docs/sdks/tenants/README.md#get_tenant_billing) - Get billing usage for the current tenant
* [post_tenants](docs/sdks/tenants/README.md#post_tenants) - Create a new tenant
* [put_tenants_update](docs/sdks/tenants/README.md#put_tenants_update) - Update a tenant
* [get_tenants_id_](docs/sdks/tenants/README.md#get_tenants_id_) - Get tenant by ID

### [Users](docs/sdks/users/README.md)

* [post_users](docs/sdks/users/README.md#post_users) - Create service account
* [get_users_me](docs/sdks/users/README.md#get_users_me) - Get user info
* [post_users_search](docs/sdks/users/README.md#post_users_search) - List users with filters

### [Wallets](docs/sdks/wallets/README.md)

* [get_customers_wallets](docs/sdks/wallets/README.md#get_customers_wallets) - Get Customer Wallets
* [get_customers_id_wallets](docs/sdks/wallets/README.md#get_customers_id_wallets) - Get wallets by customer ID
* [get_wallets](docs/sdks/wallets/README.md#get_wallets) - List wallets
* [post_wallets](docs/sdks/wallets/README.md#post_wallets) - Create a new wallet
* [post_wallets_search](docs/sdks/wallets/README.md#post_wallets_search) - List wallets by filter
* [post_wallets_transactions_search](docs/sdks/wallets/README.md#post_wallets_transactions_search) - List wallet transactions by filter
* [get_wallets_id_](docs/sdks/wallets/README.md#get_wallets_id_) - Get wallet by ID
* [put_wallets_id_](docs/sdks/wallets/README.md#put_wallets_id_) - Update a wallet
* [get_wallets_id_balance_real_time](docs/sdks/wallets/README.md#get_wallets_id_balance_real_time) - Get wallet balance
* [post_wallets_id_terminate](docs/sdks/wallets/README.md#post_wallets_id_terminate) - Terminate a wallet
* [post_wallets_id_top_up](docs/sdks/wallets/README.md#post_wallets_id_top_up) - Top up wallet
* [get_wallets_id_transactions](docs/sdks/wallets/README.md#get_wallets_id_transactions) - Get wallet transactions

### [Webhooks](docs/sdks/webhooks/README.md)

* [post_webhooks_chargebee_tenant_id_environment_id_](docs/sdks/webhooks/README.md#post_webhooks_chargebee_tenant_id_environment_id_) - Handle Chargebee webhook events
* [post_webhooks_hubspot_tenant_id_environment_id_](docs/sdks/webhooks/README.md#post_webhooks_hubspot_tenant_id_environment_id_) - Handle HubSpot webhook events
* [post_webhooks_nomod_tenant_id_environment_id_](docs/sdks/webhooks/README.md#post_webhooks_nomod_tenant_id_environment_id_) - Handle Nomod webhook events
* [post_webhooks_quickbooks_tenant_id_environment_id_](docs/sdks/webhooks/README.md#post_webhooks_quickbooks_tenant_id_environment_id_) - Handle QuickBooks webhook events
* [post_webhooks_razorpay_tenant_id_environment_id_](docs/sdks/webhooks/README.md#post_webhooks_razorpay_tenant_id_environment_id_) - Handle Razorpay webhook events
* [post_webhooks_stripe_tenant_id_environment_id_](docs/sdks/webhooks/README.md#post_webhooks_stripe_tenant_id_environment_id_) - Handle Stripe webhook events

</details>
<!-- End Available Resources and Operations [operations] -->

<!-- Start File uploads [file-upload] -->
## File uploads

Certain SDK methods accept file objects as part of a request body or multi-part request. It is possible and typically recommended to upload files as a stream rather than reading the entire contents into memory. This avoids excessive memory consumption and potentially crashing with out-of-memory errors when working with very large files. The following example demonstrates how to attach a file stream to a request.

> [!TIP]
>
> For endpoints that handle file uploads bytes arrays can also be used. However, using streams is recommended for large files.
>

```python
from flexprice_sdk_test import FlexPrice


with FlexPrice(
    server_url="https://api.example.com",
    api_key_auth="<YOUR_API_KEY_HERE>",
) as flex_price:

    res = flex_price.events.post_events_analytics(request=open("example.file", "rb"))

    # Handle response
    print(res)

```
<!-- End File uploads [file-upload] -->

<!-- Start Retries [retries] -->
## Retries

Some of the endpoints in this SDK support retries. If you use the SDK without any configuration, it will fall back to the default retry strategy provided by the API. However, the default retry strategy can be overridden on a per-operation basis, or across the entire SDK.

To change the default retry strategy for a single API call, simply provide a `RetryConfig` object to the call:
```python
from flexprice_sdk_test import FlexPrice
from flexprice_sdk_test.utils import BackoffStrategy, RetryConfig


with FlexPrice(
    server_url="https://api.example.com",
    api_key_auth="<YOUR_API_KEY_HERE>",
) as flex_price:

    res = flex_price.addons.get_addons(,
        RetryConfig("backoff", BackoffStrategy(1, 50, 1.1, 100), False))

    # Handle response
    print(res)

```

If you'd like to override the default retry strategy for all operations that support retries, you can use the `retry_config` optional parameter when initializing the SDK:
```python
from flexprice_sdk_test import FlexPrice
from flexprice_sdk_test.utils import BackoffStrategy, RetryConfig


with FlexPrice(
    server_url="https://api.example.com",
    retry_config=RetryConfig("backoff", BackoffStrategy(1, 50, 1.1, 100), False),
    api_key_auth="<YOUR_API_KEY_HERE>",
) as flex_price:

    res = flex_price.addons.get_addons()

    # Handle response
    print(res)

```
<!-- End Retries [retries] -->

<!-- Start Error Handling [errors] -->
## Error Handling

[`FlexPriceError`](./src/flexprice_sdk_test/models/errors/flexpriceerror.py) is the base class for all HTTP error responses. It has the following properties:

| Property           | Type             | Description                                                                             |
| ------------------ | ---------------- | --------------------------------------------------------------------------------------- |
| `err.message`      | `str`            | Error message                                                                           |
| `err.status_code`  | `int`            | HTTP response status code eg `404`                                                      |
| `err.headers`      | `httpx.Headers`  | HTTP response headers                                                                   |
| `err.body`         | `str`            | HTTP body. Can be empty string if no body is returned.                                  |
| `err.raw_response` | `httpx.Response` | Raw HTTP response                                                                       |
| `err.data`         |                  | Optional. Some errors may contain structured data. [See Error Classes](#error-classes). |

### Example
```python
from flexprice_sdk_test import FlexPrice
from flexprice_sdk_test.models import errors


with FlexPrice(
    server_url="https://api.example.com",
    api_key_auth="<YOUR_API_KEY_HERE>",
) as flex_price:
    res = None
    try:

        res = flex_price.addons.get_addons()

        # Handle response
        print(res)


    except errors.FlexPriceError as e:
        # The base class for HTTP error responses
        print(e.message)
        print(e.status_code)
        print(e.body)
        print(e.headers)
        print(e.raw_response)

        # Depending on the method different errors may be thrown
        if isinstance(e, errors.ErrorsErrorResponse):
            print(e.data.error)  # Optional[components.ErrorsErrorDetail]
            print(e.data.success)  # Optional[bool]
```

### Error Classes
**Primary errors:**
* [`FlexPriceError`](./src/flexprice_sdk_test/models/errors/flexpriceerror.py): The base class for HTTP error responses.
  * [`ErrorsErrorResponse`](./src/flexprice_sdk_test/models/errors/errorserrorresponse.py): *

<details><summary>Less common errors (5)</summary>

<br />

**Network errors:**
* [`httpx.RequestError`](https://www.python-httpx.org/exceptions/#httpx.RequestError): Base class for request errors.
    * [`httpx.ConnectError`](https://www.python-httpx.org/exceptions/#httpx.ConnectError): HTTP client was unable to make a request to a server.
    * [`httpx.TimeoutException`](https://www.python-httpx.org/exceptions/#httpx.TimeoutException): HTTP request timed out.


**Inherit from [`FlexPriceError`](./src/flexprice_sdk_test/models/errors/flexpriceerror.py)**:
* [`ResponseValidationError`](./src/flexprice_sdk_test/models/errors/responsevalidationerror.py): Type mismatch between the response data and the expected Pydantic model. Provides access to the Pydantic validation error via the `cause` attribute.

</details>

\* Check [the method documentation](#available-resources-and-operations) to see if the error is applicable.
<!-- End Error Handling [errors] -->

<!-- Start Custom HTTP Client [http-client] -->
## Custom HTTP Client

The Python SDK makes API calls using the [httpx](https://www.python-httpx.org/) HTTP library.  In order to provide a convenient way to configure timeouts, cookies, proxies, custom headers, and other low-level configuration, you can initialize the SDK client with your own HTTP client instance.
Depending on whether you are using the sync or async version of the SDK, you can pass an instance of `HttpClient` or `AsyncHttpClient` respectively, which are Protocol's ensuring that the client has the necessary methods to make API calls.
This allows you to wrap the client with your own custom logic, such as adding custom headers, logging, or error handling, or you can just pass an instance of `httpx.Client` or `httpx.AsyncClient` directly.

For example, you could specify a header for every request that this sdk makes as follows:
```python
from flexprice_sdk_test import FlexPrice
import httpx

http_client = httpx.Client(headers={"x-custom-header": "someValue"})
s = FlexPrice(client=http_client)
```

or you could wrap the client with your own custom logic:
```python
from flexprice_sdk_test import FlexPrice
from flexprice_sdk_test.httpclient import AsyncHttpClient
import httpx

class CustomClient(AsyncHttpClient):
    client: AsyncHttpClient

    def __init__(self, client: AsyncHttpClient):
        self.client = client

    async def send(
        self,
        request: httpx.Request,
        *,
        stream: bool = False,
        auth: Union[
            httpx._types.AuthTypes, httpx._client.UseClientDefault, None
        ] = httpx.USE_CLIENT_DEFAULT,
        follow_redirects: Union[
            bool, httpx._client.UseClientDefault
        ] = httpx.USE_CLIENT_DEFAULT,
    ) -> httpx.Response:
        request.headers["Client-Level-Header"] = "added by client"

        return await self.client.send(
            request, stream=stream, auth=auth, follow_redirects=follow_redirects
        )

    def build_request(
        self,
        method: str,
        url: httpx._types.URLTypes,
        *,
        content: Optional[httpx._types.RequestContent] = None,
        data: Optional[httpx._types.RequestData] = None,
        files: Optional[httpx._types.RequestFiles] = None,
        json: Optional[Any] = None,
        params: Optional[httpx._types.QueryParamTypes] = None,
        headers: Optional[httpx._types.HeaderTypes] = None,
        cookies: Optional[httpx._types.CookieTypes] = None,
        timeout: Union[
            httpx._types.TimeoutTypes, httpx._client.UseClientDefault
        ] = httpx.USE_CLIENT_DEFAULT,
        extensions: Optional[httpx._types.RequestExtensions] = None,
    ) -> httpx.Request:
        return self.client.build_request(
            method,
            url,
            content=content,
            data=data,
            files=files,
            json=json,
            params=params,
            headers=headers,
            cookies=cookies,
            timeout=timeout,
            extensions=extensions,
        )

s = FlexPrice(async_client=CustomClient(httpx.AsyncClient()))
```
<!-- End Custom HTTP Client [http-client] -->

<!-- Start Resource Management [resource-management] -->
## Resource Management

The `FlexPrice` class implements the context manager protocol and registers a finalizer function to close the underlying sync and async HTTPX clients it uses under the hood. This will close HTTP connections, release memory and free up other resources held by the SDK. In short-lived Python programs and notebooks that make a few SDK method calls, resource management may not be a concern. However, in longer-lived programs, it is beneficial to create a single SDK instance via a [context manager][context-manager] and reuse it across the application.

[context-manager]: https://docs.python.org/3/reference/datamodel.html#context-managers

```python
from flexprice_sdk_test import FlexPrice
def main():

    with FlexPrice(
        server_url="https://api.example.com",
        api_key_auth="<YOUR_API_KEY_HERE>",
    ) as flex_price:
        # Rest of application here...


# Or when using async:
async def amain():

    async with FlexPrice(
        server_url="https://api.example.com",
        api_key_auth="<YOUR_API_KEY_HERE>",
    ) as flex_price:
        # Rest of application here...
```
<!-- End Resource Management [resource-management] -->

<!-- Start Debugging [debug] -->
## Debugging

You can setup your SDK to emit debug logs for SDK requests and responses.

You can pass your own logger class directly into your SDK.
```python
from flexprice_sdk_test import FlexPrice
import logging

logging.basicConfig(level=logging.DEBUG)
s = FlexPrice(server_url="https://example.com", debug_logger=logging.getLogger("flexprice_sdk_test"))
```
<!-- End Debugging [debug] -->

<!-- Placeholder for Future Speakeasy SDK Sections -->
