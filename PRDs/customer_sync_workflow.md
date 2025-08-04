# Customer Synchronisation – Integration Workflow PRD

**Document version:** v1.0  
**Last updated:** 2025-08-03  
**Author:** FlexPrice Engineering

---

## 1. Purpose
Describe how FlexPrice synchronises customer objects between FlexPrice (FP) and external providers (currently Stripe), list the touch-points (API, webhooks, DB), and enumerate edge cases and error handling requirements.

## 2. Scope
* FlexPrice multi-tenant SaaS platform  
* Providers supported: Stripe (v1), extensible to Razorpay, Finix, etc.  
* Entities: `customer` (core), `entity_integration_mapping`, `connection`.

## 3. High-Level Architecture
```
+-------------+           SyncCustomerToProviders            +----------------+
|  FP  API    |  (1) -> service.CustomerSyncService  ---->  |  Provider SDK  |
|  /customers |                                                   |  (Stripe)   |
+-------------+                                               /- |              |
        ^                                                    /   +----------------+
        |  Webhook (customer.created/updated/deleted)       /
        |                                                   /
+-------------+   (4)   webhook.StripeService.HandleEvent  /
|  FP Webhook | <-----------------------------------------/
| /v1/webhooks |
+-------------+
```
Numbers:
1. Tenant creates/updates a FP customer; `CustomerService` publishes webhook & calls `CustomerSyncService.SyncCustomerToProviders`.
2. `IntegrationService` filters active `connection`s and invokes provider-specific service (Stripe) -> **push**.
3. Provider (Stripe) emits webhook; `WebhookHandler` delegates to `CustomerSyncService.SyncCustomerFromProvider` -> **pull**.
4. Entity mapping table keeps 1-to-N mapping between FP IDs and provider IDs.

## 4. Detailed Flow
### 4.1 Outbound (FP → Provider)
1. `CustomerService.CreateCustomer` validates & saves FP customer.  
2. Publishes `customer.created` internal webhook.
3. Async goroutine triggers `CustomerSyncService.SyncCustomerToProviders`.
4. `IntegrationService` loads tenant’s **active** `connection`s.
5. For each connection:
   1. Loads & decrypts provider credentials.
   2. Calls `StripeService.CreateCustomerInStripeWithConfig`.
   3. On success – persists `EntityIntegrationMapping` with metadata `{connection_id, synced_at}`.
6. Failures logged and retried via background job (future work).

### 4.2 Inbound (Provider → FP)
1. Stripe webhook hits `/v1/webhooks/stripe/{tenant_id}/{environment_id}`.
2. `WebhookHandler` verifies signature & delegates to `StripeService`.
3. `StripeService.handleCustomerCreated/Updated/Deleted` converts payload → `CustomerSyncService.SyncCustomerFromProvider`.
4. Service upserts FP customer, updates mapping, and republishes `customer.*` internal webhooks.

## 5. Data Model
| Table | Key Fields |
|-------|------------|
| `customer` | `id`, `email`, `metadata->stripe_customer_id` |
| `entity_integration_mapping` | `entity_id`, `provider_entity_id`, `provider_type`, `connection_id` |
| `connection` | `id`, `metadata(stripe.secret_key, webhook_secret)` |

## 6. Webhook Events Used
| Stripe Event | Purpose |
|--------------|---------|
| `customer.created` | Create FP customer if not present |
| `customer.updated` | Sync updates (email, address, name) |
| `customer.deleted` | [Soft] delete or mark inactive in FP |

## 7. Edge Cases & Error Handling
| # | Scenario | Expected Behaviour |
|---|----------|--------------------|
| 1 | Duplicate webhook delivery | Idempotent via `idempotency_key` + mapping uniqueness |
| 2 | Connection secret rotated | Decryption failure → mark connection **inactive**, alert Ops |
| 3 | Stripe rate-limit (429) | Exponential back-off (future retry queue) |
| 4 | Customer exists in FP with same email but no mapping | Link instead of create new; configurable conflict strategy |
| 5 | Provider down | Retry with jitter; after N failures raise incident |
| 6 | Mapping deleted accidentally | Re-create on next outbound sync |
| 7 | Tenant has zero active connections | Log & skip sync silently |

## 8. Non-Functional Requirements
* Throughput: ≥ 100 rps sync bursts.
* Latency: 95p < 2 s per sync call.
* Data consistency: Eventually consistent (< 1 min) between FP & provider.
* Security: AES-GCM encryption for connection metadata; HMAC webhook signature.

## 9. Monitoring & Alerts
* Counters: sync success/failure, webhook failures, mapping conflicts.
* Dashboards: Grafana panels per tenant.
* Alert rules: >5% sync failures over 10 min OR webhook verification errors.

## 10. Future Enhancements
* Generic retry/DEAD-LETTER queue.
* Add support for other providers via factory pattern.
* Bulk backfill utility.

---
END
