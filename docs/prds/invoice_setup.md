### Product Requirements Document: Invoice Entity for Flexprice

#### Overview

The Invoice entity is a core component to enable detailed billing for Flexprice. It will handle generating invoices for various use cases, including subscriptions, wallet top-ups, and manual invoice generation. This document outlines the technical requirements and functionalities of the Invoice entity to ensure alignment with Flexprice’s goals and existing architecture.

---

### Objectives

1. Provide a structured invoice entity to track charges and payments.
2. Enable both manual and automated invoice creation workflows.
3. Support multiple states for invoice lifecycle management (e.g., draft, finalized, paid, voided).
4. Integrate seamlessly with existing subscription workflows to automate invoice generation during renewals or billing events.
5. Support updates to payment statuses, voiding invoices, and capturing edge cases (e.g., proration, currency mismatch).
6. Provide API endpoints for creating, retrieving, updating, and managing invoices.

---

### Functional Requirements

#### Invoice Use Cases

1. **Subscription Billing:** Automatically generate invoices during subscription renewals or at the start of a new billing period.
2. **Wallet Top-Ups:** Generate invoices when a customer adds credits to their wallet.
3. **Manual Invoices:** Allow users to create invoices manually via APIs for one-time charges or adjustments.

#### Invoice States

- **Draft:** Initial state for invoices that can be modified.
- **Finalized:** Indicates the invoice is ready to be sent to the customer.
- **Paid:** Indicates payment has been received.
- **Voided:** Represents canceled or invalid invoices.

#### Supported Actions

1. **Create Invoice:** Ability to create invoices manually or programmatically during subscription workflows.
2. **Update Payment Status:** Update the status to reflect payments (e.g., partial, fully paid).
3. **Void Invoice:** Mark an invoice as voided, ensuring no further action.
4. **Fetch Invoice Details:** Retrieve detailed invoice information.
5. **List Invoices:** Fetch a list of invoices filtered by customer, subscription, or status.

---

### Data Model

The Invoice entity will include the following fields:

| Field             | Type         | Description                                               |
| ----------------- | ------------ | --------------------------------------------------------- |
| `id`              | `string`     | Unique identifier for the invoice                         |
| `customer_id`     | `string`     | ID of the customer associated with the invoice            |
| `subscription_id` | `string`     | (Optional) ID of the subscription related to this invoice |
| `wallet_id`       | `string`     | (Optional) ID of the wallet related to this invoice       |
| `status`          | `enum`       | Draft, Finalized, Paid, Voided                            |
| `amount_due`      | `decimal`    | Total amount due                                          |
| `currency`        | `string`     | Currency code (e.g., USD, EUR)                            |
| `amount_paid`     | `decimal`    | Total amount paid                                         |
| `created_at`      | `datetime`   | Timestamp of invoice creation                             |
| `updated_at`      | `datetime`   | Timestamp of last update                                  |
| `due_date`        | `datetime`   | Due date for payment                                      |
| `line_items`      | `[]LineItem` | List of line items in the invoice                         |

#### LineItem Structure

| Field         | Type      | Description                         |
| ------------- | --------- | ----------------------------------- |
| `id`          | `string`  | Unique identifier for the line item |
| `description` | `string`  | Description of the charge           |
| `amount`      | `decimal` | Amount for this line item           |
| `quantity`    | `int`     | Quantity                            |
| `total`       | `decimal` | Total amount (amount x quantity)    |

---

### API Endpoints

#### Create Invoice

**POST** `/v1/invoices`

- Request:

```json
{
  "customer_id": "cus_123",
  "subscription_id": "sub_123",
  "wallet_id": "wallet_123",
  "due_date": "2024-01-01T00:00:00Z",
  "line_items": [
    {
      "description": "API usage",
      "amount": 50,
      "quantity": 2
    }
  ]
}
```

- Response:

```json
{
  "id": "inv_123",
  "status": "draft",
  "amount_due": 100,
  "currency": "usd",
  "created_at": "2023-12-28T00:00:00Z"
}
```

#### Fetch Invoice

**GET** `/v1/invoices/{id}`

- Response:

```json
{
  "id": "inv_123",
  "status": "finalized",
  "customer_id": "cus_123",
  "amount_due": 100,
  "amount_paid": 0,
  "line_items": [
    {
      "id": "li_456",
      "description": "API usage",
      "amount": 50,
      "quantity": 2,
      "total": 100
    }
  ]
}
```

#### Update Invoice Payment Status

**PUT** `/v1/invoices/{id}/payment`

- Request:

```json
{
  "amount_paid": 100
}
```

- Response:

```json
{
  "id": "inv_123",
  "status": "paid",
  "amount_due": 0,
  "amount_paid": 100
}
```

#### Void Invoice

**POST** `/v1/invoices/{id}/void`

- Response:

```json
{
  "id": "inv_123",
  "status": "voided"
}
```

#### List Invoices

**GET** `/v1/invoices`

- Query Parameters: `customer_id`, `status`, `subscription_id`
- Response:

```json
[
  {
    "id": "inv_123",
    "status": "draft",
    "amount_due": 100,
    "currency": "usd",
    "created_at": "2023-12-28T00:00:00Z"
  }
]
```

---

### Integration Points

1. **Subscription Workflows:**

   - Auto-generate invoices during subscription renewals or billing period changes.
   - Attach line items based on aggregated usage and subscription details.

2. **Payment Handling:**

   - Update invoice payment status upon payment capture.
   - Void invoices for canceled subscriptions.

3. **Error Handling:**

   - Validate invoice creation requests for required fields (e.g., `customer_id`, `due_date`).
   - Handle scenarios like partial payments or mismatched currencies.

---

### Future Enhancements

1. Add support for tax calculations and discounts.
2. Introduce email notifications for customers upon invoice finalization.
3. Enable PDF generation for invoices.
4. Build reporting capabilities to track invoicing and payment trends.

---

### Conclusion

The Invoice entity will serve as a foundational component in Flexprice’s billing and invoicing workflows. With robust integration into existing systems, it will simplify tracking and managing billing for customers, subscriptions, and other financial activities.
