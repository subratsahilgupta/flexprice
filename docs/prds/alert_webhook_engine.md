# Alert Webhook Engine Design Document

## Overview

The Alert Engine is a standalone system designed to handle threshold-based alerts across different components of the FlexPrice platform. The system implements a simplified three-level threshold hierarchy (Endpoint -> Entity -> Tenant) with clear precedence rules. Alerts can be triggered via API endpoints and scheduled checks.

The system introduces a dedicated Alert table to store threshold configurations and alert states, making it more efficient to manage and track alerts across the platform. When thresholds are provided at the endpoint level, the system bypasses checking entity and tenant level thresholds, optimizing the evaluation process.

## Core Concepts

### 1. Entity Types and Metrics

Each alert in the system is associated with a specific entity type and metric combination. These are defined using the `EntityMetric` type to ensure type safety and validation.

```go
// Entity Types and Metrics are defined in types.EntityMetric or model.go
type EntityMetric string

const (
    // Wallet Entity Metrics
    WalletOngoingBalance  EntityMetric = "wallet.ongoing_balance"
    WalletCurrentBalance  EntityMetric = "wallet.current_balance"
    
    // Entitlement Entity Metrics
    EntitlementUsage      EntityMetric = "entitlement.usage"
    EntitlementLimit      EntityMetric = "entitlement.limit"
    
    // Add other entity types and metrics as needed
)
```

Each entity type can have multiple metrics that can be monitored. For example:
- Wallet entity can monitor ongoing_balance, current_balance
- Entitlement entity can monitor usage, limit
- Each new alert requirement will be added as a new entity type with its specific metrics

### 2. Configuration Models

```go
// Alert Check Request
type AlertCheckRequest struct {
    TenantIDs []string `json:"tenant_ids"`
    EnvIDs    []string `json:"env_ids"`
    Entity    Entity   `json:"entity"`
    CheckAgainstThreshold *Threshold `json:"check_against_threshold,omitempty"`
}

// Entity Definition
type Entity struct {
    Type   EntityMetric `json:"type"`         // type.entity_metric
    Metric EntityMetric `json:"entity_metric"` // type.entity_metric
}

// Threshold Configuration
type Threshold struct {
    Value    decimal.Decimal `json:"value"`
    Operator string         `json:"operator"` // gt, lt, etc.
}

// Entity Level Threshold Request
type EntityThresholdRequest struct {
    Entity    Entity    `json:"entity"`
    EntityID  string    `json:"entity_id"`
    Threshold Threshold `json:"set_threshold_entityLevel"`
}

// Tenant Level Threshold Request
type TenantThresholdRequest struct {
    TenantIDs []string  `json:"tenant_ids"`
    EnvIDs    []string  `json:"env_ids"`
    Entity    Entity    `json:"entity"`
    Threshold Threshold `json:"set_threshold_tenantLevel"`
}

// Alert Table Schema
type Alert struct {
    ID                string         `json:"id"`
    TenantID         string         `json:"tenant_id"`
    EnvID            string         `json:"env_id"`
    EntityType       string         `json:"entity_type"`
    Metric           string         `json:"entity_metric"`
    EntityID         string         `json:"entity_id"`
    EntityThreshold  *Threshold     `json:"entity_threshold"`
    TenantThreshold  *Threshold     `json:"tenant_threshold"`
    AlertEnabled     bool           `json:"alert_enabled"`
    AlertState       AlertState     `json:"alert_state"`
    Metadata         interface{}    `json:"metadata,omitempty"`
}

// Alert States
type AlertState string

const (
    AlertStateOK      AlertState = "ok"
    AlertStateInAlarm AlertState = "in_alarm"
)

// Webhook Events
const (
    WebhookEventAlertTriggered = "alert.triggered"
    WebhookEventAlertRecovered = "alert.recovered"
)

// Alert Webhook Payload
type AlertWebhookPayload struct {
    EntityID     string         `json:"entity_id"`
    EntityType   string         `json:"entity_type"` 
    TenantID     string         `json:"tenant_id"`
    EnvID        string         `json:"env_id"`
    AlertState   string         `json:"alert_state"`
    Threshold    Threshold      `json:"threshold"`
    CurrentValue interface{}    `json:"current_value"`
    Inheritance  string         `json:"inheritance"` // endpoint, entity, tenant
}
```

### 3. Alert Processing Flow

```mermaid
sequenceDiagram
    participant EP as Endpoint
    participant AM as Alert Manager
    participant CR as Config Resolver
    participant E as Evaluator
    participant W as Webhook

    Note over EP: Can be Cron Job or<br/>API Request
    
    EP->>AM: Check Alert(tenantIDs, envIDs, entity, threshold)
    AM->>CR: Get Config Priority
    Note over CR: 1. Endpoint Level (if provided)<br/>2. Entity Level<br/>3. Tenant Level
    CR-->>AM: Effective Config
    
    AM->>E: Evaluate Entity
    E-->>AM: Evaluation Result
    
    alt Threshold Breached
        AM->>AM: Update Alert State
        AM->>W: Publish Alert
    end
```

### 4. Evaluation Flow

```mermaid
graph TD
    A[Endpoint Trigger<br/>Cron/API] --> B[Get Alert Config]
    B --> C{Config<br/>Resolution}
    
    C -->|1st| D[Check Endpoint<br/>Threshold]
    C -->|2nd| E[Check Entity<br/>Config]
    C -->|3rd| F[Check Tenant<br/>Config]
    
    D & E & F --> G[Evaluate<br/>Threshold]
    
    G --> H{Threshold<br/>Breached?}
    H -->|Yes| I[Update Alert State]
    I --> J[Send Webhook]
```

## Configuration Resolution

### 1. Priority Rules

1. **Endpoint Level (Highest)**
   - Set via API request with check_against_threshold
   - Overrides all other configurations
   - Used for one-time checks or scheduled evaluations

2. **Entity Level**
   - Entity-specific configuration
   - Stored in Alert table
   - Used if no endpoint threshold provided

3. **Tenant Level**
   - Tenant-wide configuration
   - Stored in Alert table
   - Used if no endpoint threshold and no entity config

### 2. Resolution Process

```go
func (r *ConfigResolver) ResolveConfig(ctx context.Context, req AlertCheckRequest) (*Threshold, error) {
    // 1. Check endpoint threshold (highest priority)
    if req.CheckAgainstThreshold != nil {
        return req.CheckAgainstThreshold, nil
    }

    // 2. Check entity level config
    if entityThreshold := r.alertRepo.GetEntityThreshold(req.Entity.Type, req.Entity.Metric, req.EntityID); entityThreshold != nil {
        return entityThreshold, nil
    }

    // 3. Check tenant level config
    if tenantThreshold := r.alertRepo.GetTenantThreshold(req.TenantIDs[0], req.Entity.Type, req.Entity.Metric); tenantThreshold != nil {
        return tenantThreshold, nil
    }

    return nil, errors.New("no alert configuration found")
}
```

### 3. Webhook Events

```go
const (
    WebhookEventAlertTriggered = "alert.triggered"
    WebhookEventAlertRecovered = "alert.recovered"
)

type AlertWebhookPayload struct {
    EntityID     string         `json:"entity_id"`
    EntityType   string         `json:"entity_type"` // wallet, entitlement, etc.
    TenantID     string         `json:"tenant_id"`
    EnvID        string         `json:"env_id"`
    AlertState   string         `json:"alert_state"`
    Threshold    Threshold      `json:"threshold"`
    CurrentValue interface{}    `json:"current_value"`
    Inheritance  string         `json:"inheritance"` // endpoint, entity, tenant
}
```

## Implementation Components

### 1. Alert Manager
- Handles alert check requests
- Coordinates configuration resolution
- Manages alert state transitions
- Triggers webhook notifications

### 2. Config Resolver
- Implements configuration precedence rules
- Resolves effective configuration
- Handles tenant and entity level configs

### 3. Evaluator
- Performs threshold comparisons
- Supports different operators (gt, lt, etc.)
- Handles value normalization

### 4. Webhook Publisher
- Delivers alert notifications
- Manages retry logic
- Handles webhook signatures

## Monitoring and Observability

### 1. Metrics
- Alert trigger count by type and level
- Configuration resolution stats
- Evaluation latency
- Webhook delivery success rate

### 2. Logging
```go
logger.Infow("alert evaluated",
    "entity_id", entity.ID,
    "entity_type", entity.Type,
    "tenant_id", entity.TenantID,
    "config_inheritance", config.Inheritance,
    "threshold", config.Threshold,
    "current_value", value,
    "alert_state", newState,
)
```

## Future Enhancements

1. **Advanced Configuration**
   - Multiple thresholds per entity
   - Time-based thresholds
   - Custom evaluation logic

2. **Notification Channels**
   - Email notifications
   - SMS alerts
   - Slack integration

3. **Analytics**
   - Alert frequency analysis
   - Pattern detection
   - Threshold optimization

## Visuals

### _Workflow_
<svg aria-roledescription="sequence" role="graphics-document document" viewBox="-50 -10 1235 745" style="max-width: 1235px;" xmlns="http://www.w3.org/2000/svg" width="100%" id="mermaid-svg-1754894741246-9sf373sk7"><g><rect class="actor" ry="3" rx="3" height="65" width="150" stroke="#666" fill="#eaeaea" y="659" x="985"/><text style="text-anchor: middle; font-size: 16px; font-weight: 400;" class="actor" alignment-baseline="central" dominant-baseline="central" y="691.5" x="1060"><tspan dy="0" x="1060">Webhook</tspan></text></g><g><rect class="actor" ry="3" rx="3" height="65" width="150" stroke="#666" fill="#eaeaea" y="659" x="785"/><text style="text-anchor: middle; font-size: 16px; font-weight: 400;" class="actor" alignment-baseline="central" dominant-baseline="central" y="691.5" x="860"><tspan dy="0" x="860">Evaluator</tspan></text></g><g><rect class="actor" ry="3" rx="3" height="65" width="150" stroke="#666" fill="#eaeaea" y="659" x="585"/><text style="text-anchor: middle; font-size: 16px; font-weight: 400;" class="actor" alignment-baseline="central" dominant-baseline="central" y="691.5" x="660"><tspan dy="0" x="660">Config Resolver</tspan></text></g><g><rect class="actor" ry="3" rx="3" height="65" width="150" stroke="#666" fill="#eaeaea" y="659" x="385"/><text style="text-anchor: middle; font-size: 16px; font-weight: 400;" class="actor" alignment-baseline="central" dominant-baseline="central" y="691.5" x="460"><tspan dy="0" x="460">Alert Manager</tspan></text></g><g><rect class="actor" ry="3" rx="3" height="65" width="150" stroke="#666" fill="#eaeaea" y="659" x="0"/><text style="text-anchor: middle; font-size: 16px; font-weight: 400;" class="actor" alignment-baseline="central" dominant-baseline="central" y="691.5" x="75"><tspan dy="0" x="75">Endpoint</tspan></text></g><g><line stroke="#999" stroke-width="0.5px" class="200" y2="659" x2="1060" y1="5" x1="1060" id="actor143"/><g id="root-143"><rect class="actor" ry="3" rx="3" height="65" width="150" stroke="#666" fill="#eaeaea" y="0" x="985"/><text style="text-anchor: middle; font-size: 16px; font-weight: 400;" class="actor" alignment-baseline="central" dominant-baseline="central" y="32.5" x="1060"><tspan dy="0" x="1060">Webhook</tspan></text></g></g><g><line stroke="#999" stroke-width="0.5px" class="200" y2="659" x2="860" y1="5" x1="860" id="actor142"/><g id="root-142"><rect class="actor" ry="3" rx="3" height="65" width="150" stroke="#666" fill="#eaeaea" y="0" x="785"/><text style="text-anchor: middle; font-size: 16px; font-weight: 400;" class="actor" alignment-baseline="central" dominant-baseline="central" y="32.5" x="860"><tspan dy="0" x="860">Evaluator</tspan></text></g></g><g><line stroke="#999" stroke-width="0.5px" class="200" y2="659" x2="660" y1="5" x1="660" id="actor141"/><g id="root-141"><rect class="actor" ry="3" rx="3" height="65" width="150" stroke="#666" fill="#eaeaea" y="0" x="585"/><text style="text-anchor: middle; font-size: 16px; font-weight: 400;" class="actor" alignment-baseline="central" dominant-baseline="central" y="32.5" x="660"><tspan dy="0" x="660">Config Resolver</tspan></text></g></g><g><line stroke="#999" stroke-width="0.5px" class="200" y2="659" x2="460" y1="5" x1="460" id="actor140"/><g id="root-140"><rect class="actor" ry="3" rx="3" height="65" width="150" stroke="#666" fill="#eaeaea" y="0" x="385"/><text style="text-anchor: middle; font-size: 16px; font-weight: 400;" class="actor" alignment-baseline="central" dominant-baseline="central" y="32.5" x="460"><tspan dy="0" x="460">Alert Manager</tspan></text></g></g><g><line stroke="#999" stroke-width="0.5px" class="200" y2="659" x2="75" y1="5" x1="75" id="actor139"/><g id="root-139"><rect class="actor" ry="3" rx="3" height="65" width="150" stroke="#666" fill="#eaeaea" y="0" x="0"/><text style="text-anchor: middle; font-size: 16px; font-weight: 400;" class="actor" alignment-baseline="central" dominant-baseline="central" y="32.5" x="75"><tspan dy="0" x="75">Endpoint</tspan></text></g></g><style>#mermaid-svg-1754894741246-9sf373sk7{font-family:"trebuchet ms",verdana,arial,sans-serif;font-size:16px;fill:rgba(204, 204, 204, 0.87);}#mermaid-svg-1754894741246-9sf373sk7 .error-icon{fill:#bf616a;}#mermaid-svg-1754894741246-9sf373sk7 .error-text{fill:#bf616a;stroke:#bf616a;}#mermaid-svg-1754894741246-9sf373sk7 .edge-thickness-normal{stroke-width:2px;}#mermaid-svg-1754894741246-9sf373sk7 .edge-thickness-thick{stroke-width:3.5px;}#mermaid-svg-1754894741246-9sf373sk7 .edge-pattern-solid{stroke-dasharray:0;}#mermaid-svg-1754894741246-9sf373sk7 .edge-pattern-dashed{stroke-dasharray:3;}#mermaid-svg-1754894741246-9sf373sk7 .edge-pattern-dotted{stroke-dasharray:2;}#mermaid-svg-1754894741246-9sf373sk7 .marker{fill:rgba(204, 204, 204, 0.87);stroke:rgba(204, 204, 204, 0.87);}#mermaid-svg-1754894741246-9sf373sk7 .marker.cross{stroke:rgba(204, 204, 204, 0.87);}#mermaid-svg-1754894741246-9sf373sk7 svg{font-family:"trebuchet ms",verdana,arial,sans-serif;font-size:16px;}#mermaid-svg-1754894741246-9sf373sk7 .actor{stroke:hsl(210, 0%, 73.137254902%);fill:#81a1c1;}#mermaid-svg-1754894741246-9sf373sk7 text.actor&gt;tspan{fill:#191c22;stroke:none;}#mermaid-svg-1754894741246-9sf373sk7 .actor-line{stroke:rgba(204, 204, 204, 0.87);}#mermaid-svg-1754894741246-9sf373sk7 .messageLine0{stroke-width:1.5;stroke-dasharray:none;stroke:rgba(204, 204, 204, 0.87);}#mermaid-svg-1754894741246-9sf373sk7 .messageLine1{stroke-width:1.5;stroke-dasharray:2,2;stroke:rgba(204, 204, 204, 0.87);}#mermaid-svg-1754894741246-9sf373sk7 #arrowhead path{fill:rgba(204, 204, 204, 0.87);stroke:rgba(204, 204, 204, 0.87);}#mermaid-svg-1754894741246-9sf373sk7 .sequenceNumber{fill:rgba(204, 204, 204, 0.61);}#mermaid-svg-1754894741246-9sf373sk7 #sequencenumber{fill:rgba(204, 204, 204, 0.87);}#mermaid-svg-1754894741246-9sf373sk7 #crosshead path{fill:rgba(204, 204, 204, 0.87);stroke:rgba(204, 204, 204, 0.87);}#mermaid-svg-1754894741246-9sf373sk7 .messageText{fill:rgba(204, 204, 204, 0.87);stroke:none;}#mermaid-svg-1754894741246-9sf373sk7 .labelBox{stroke:#454545;fill:#141414;}#mermaid-svg-1754894741246-9sf373sk7 .labelText,#mermaid-svg-1754894741246-9sf373sk7 .labelText&gt;tspan{fill:rgba(204, 204, 204, 0.87);stroke:none;}#mermaid-svg-1754894741246-9sf373sk7 .loopText,#mermaid-svg-1754894741246-9sf373sk7 .loopText&gt;tspan{fill:#d8dee9;stroke:none;}#mermaid-svg-1754894741246-9sf373sk7 .loopLine{stroke-width:2px;stroke-dasharray:2,2;stroke:#454545;fill:#454545;}#mermaid-svg-1754894741246-9sf373sk7 .note{stroke:#2a2a2a;fill:#1a1a1a;}#mermaid-svg-1754894741246-9sf373sk7 .noteText,#mermaid-svg-1754894741246-9sf373sk7 .noteText&gt;tspan{fill:rgba(204, 204, 204, 0.87);stroke:none;}#mermaid-svg-1754894741246-9sf373sk7 .activation0{fill:rgba(64, 64, 64, 0.47);stroke:#30373a;}#mermaid-svg-1754894741246-9sf373sk7 .activation1{fill:rgba(64, 64, 64, 0.47);stroke:#30373a;}#mermaid-svg-1754894741246-9sf373sk7 .activation2{fill:rgba(64, 64, 64, 0.47);stroke:#30373a;}#mermaid-svg-1754894741246-9sf373sk7 .actorPopupMenu{position:absolute;}#mermaid-svg-1754894741246-9sf373sk7 .actorPopupMenuPanel{position:absolute;fill:#81a1c1;box-shadow:0px 8px 16px 0px rgba(0,0,0,0.2);filter:drop-shadow(3px 5px 2px rgb(0 0 0 / 0.4));}#mermaid-svg-1754894741246-9sf373sk7 .actor-man line{stroke:hsl(210, 0%, 73.137254902%);fill:#81a1c1;}#mermaid-svg-1754894741246-9sf373sk7 .actor-man circle,#mermaid-svg-1754894741246-9sf373sk7 line{stroke:hsl(210, 0%, 73.137254902%);fill:#81a1c1;stroke-width:2px;}#mermaid-svg-1754894741246-9sf373sk7 :root{--mermaid-font-family:"trebuchet ms",verdana,arial,sans-serif;}</style><g/><defs><symbol height="24" width="24" id="computer"><path d="M2 2v13h20v-13h-20zm18 11h-16v-9h16v9zm-10.228 6l.466-1h3.524l.467 1h-4.457zm14.228 3h-24l2-6h2.104l-1.33 4h18.45l-1.297-4h2.073l2 6zm-5-10h-14v-7h14v7z" transform="scale(.5)"/></symbol></defs><defs><symbol clip-rule="evenodd" fill-rule="evenodd" id="database"><path d="M12.258.001l.256.004.255.005.253.008.251.01.249.012.247.015.246.016.242.019.241.02.239.023.236.024.233.027.231.028.229.031.225.032.223.034.22.036.217.038.214.04.211.041.208.043.205.045.201.046.198.048.194.05.191.051.187.053.183.054.18.056.175.057.172.059.168.06.163.061.16.063.155.064.15.066.074.033.073.033.071.034.07.034.069.035.068.035.067.035.066.035.064.036.064.036.062.036.06.036.06.037.058.037.058.037.055.038.055.038.053.038.052.038.051.039.05.039.048.039.047.039.045.04.044.04.043.04.041.04.04.041.039.041.037.041.036.041.034.041.033.042.032.042.03.042.029.042.027.042.026.043.024.043.023.043.021.043.02.043.018.044.017.043.015.044.013.044.012.044.011.045.009.044.007.045.006.045.004.045.002.045.001.045v17l-.001.045-.002.045-.004.045-.006.045-.007.045-.009.044-.011.045-.012.044-.013.044-.015.044-.017.043-.018.044-.02.043-.021.043-.023.043-.024.043-.026.043-.027.042-.029.042-.03.042-.032.042-.033.042-.034.041-.036.041-.037.041-.039.041-.04.041-.041.04-.043.04-.044.04-.045.04-.047.039-.048.039-.05.039-.051.039-.052.038-.053.038-.055.038-.055.038-.058.037-.058.037-.06.037-.06.036-.062.036-.064.036-.064.036-.066.035-.067.035-.068.035-.069.035-.07.034-.071.034-.073.033-.074.033-.15.066-.155.064-.16.063-.163.061-.168.06-.172.059-.175.057-.18.056-.183.054-.187.053-.191.051-.194.05-.198.048-.201.046-.205.045-.208.043-.211.041-.214.04-.217.038-.22.036-.223.034-.225.032-.229.031-.231.028-.233.027-.236.024-.239.023-.241.02-.242.019-.246.016-.247.015-.249.012-.251.01-.253.008-.255.005-.256.004-.258.001-.258-.001-.256-.004-.255-.005-.253-.008-.251-.01-.249-.012-.247-.015-.245-.016-.243-.019-.241-.02-.238-.023-.236-.024-.234-.027-.231-.028-.228-.031-.226-.032-.223-.034-.22-.036-.217-.038-.214-.04-.211-.041-.208-.043-.204-.045-.201-.046-.198-.048-.195-.05-.19-.051-.187-.053-.184-.054-.179-.056-.176-.057-.172-.059-.167-.06-.164-.061-.159-.063-.155-.064-.151-.066-.074-.033-.072-.033-.072-.034-.07-.034-.069-.035-.068-.035-.067-.035-.066-.035-.064-.036-.063-.036-.062-.036-.061-.036-.06-.037-.058-.037-.057-.037-.056-.038-.055-.038-.053-.038-.052-.038-.051-.039-.049-.039-.049-.039-.046-.039-.046-.04-.044-.04-.043-.04-.041-.04-.04-.041-.039-.041-.037-.041-.036-.041-.034-.041-.033-.042-.032-.042-.03-.042-.029-.042-.027-.042-.026-.043-.024-.043-.023-.043-.021-.043-.02-.043-.018-.044-.017-.043-.015-.044-.013-.044-.012-.044-.011-.045-.009-.044-.007-.045-.006-.045-.004-.045-.002-.045-.001-.045v-17l.001-.045.002-.045.004-.045.006-.045.007-.045.009-.044.011-.045.012-.044.013-.044.015-.044.017-.043.018-.044.02-.043.021-.043.023-.043.024-.043.026-.043.027-.042.029-.042.03-.042.032-.042.033-.042.034-.041.036-.041.037-.041.039-.041.04-.041.041-.04.043-.04.044-.04.046-.04.046-.039.049-.039.049-.039.051-.039.052-.038.053-.038.055-.038.056-.038.057-.037.058-.037.06-.037.061-.036.062-.036.063-.036.064-.036.066-.035.067-.035.068-.035.069-.035.07-.034.072-.034.072-.033.074-.033.151-.066.155-.064.159-.063.164-.061.167-.06.172-.059.176-.057.179-.056.184-.054.187-.053.19-.051.195-.05.198-.048.201-.046.204-.045.208-.043.211-.041.214-.04.217-.038.22-.036.223-.034.226-.032.228-.031.231-.028.234-.027.236-.024.238-.023.241-.02.243-.019.245-.016.247-.015.249-.012.251-.01.253-.008.255-.005.256-.004.258-.001.258.001zm-9.258 20.499v.01l.001.021.003.021.004.022.005.021.006.022.007.022.009.023.01.022.011.023.012.023.013.023.015.023.016.024.017.023.018.024.019.024.021.024.022.025.023.024.024.025.052.049.056.05.061.051.066.051.07.051.075.051.079.052.084.052.088.052.092.052.097.052.102.051.105.052.11.052.114.051.119.051.123.051.127.05.131.05.135.05.139.048.144.049.147.047.152.047.155.047.16.045.163.045.167.043.171.043.176.041.178.041.183.039.187.039.19.037.194.035.197.035.202.033.204.031.209.03.212.029.216.027.219.025.222.024.226.021.23.02.233.018.236.016.24.015.243.012.246.01.249.008.253.005.256.004.259.001.26-.001.257-.004.254-.005.25-.008.247-.011.244-.012.241-.014.237-.016.233-.018.231-.021.226-.021.224-.024.22-.026.216-.027.212-.028.21-.031.205-.031.202-.034.198-.034.194-.036.191-.037.187-.039.183-.04.179-.04.175-.042.172-.043.168-.044.163-.045.16-.046.155-.046.152-.047.148-.048.143-.049.139-.049.136-.05.131-.05.126-.05.123-.051.118-.052.114-.051.11-.052.106-.052.101-.052.096-.052.092-.052.088-.053.083-.051.079-.052.074-.052.07-.051.065-.051.06-.051.056-.05.051-.05.023-.024.023-.025.021-.024.02-.024.019-.024.018-.024.017-.024.015-.023.014-.024.013-.023.012-.023.01-.023.01-.022.008-.022.006-.022.006-.022.004-.022.004-.021.001-.021.001-.021v-4.127l-.077.055-.08.053-.083.054-.085.053-.087.052-.09.052-.093.051-.095.05-.097.05-.1.049-.102.049-.105.048-.106.047-.109.047-.111.046-.114.045-.115.045-.118.044-.12.043-.122.042-.124.042-.126.041-.128.04-.13.04-.132.038-.134.038-.135.037-.138.037-.139.035-.142.035-.143.034-.144.033-.147.032-.148.031-.15.03-.151.03-.153.029-.154.027-.156.027-.158.026-.159.025-.161.024-.162.023-.163.022-.165.021-.166.02-.167.019-.169.018-.169.017-.171.016-.173.015-.173.014-.175.013-.175.012-.177.011-.178.01-.179.008-.179.008-.181.006-.182.005-.182.004-.184.003-.184.002h-.37l-.184-.002-.184-.003-.182-.004-.182-.005-.181-.006-.179-.008-.179-.008-.178-.01-.176-.011-.176-.012-.175-.013-.173-.014-.172-.015-.171-.016-.17-.017-.169-.018-.167-.019-.166-.02-.165-.021-.163-.022-.162-.023-.161-.024-.159-.025-.157-.026-.156-.027-.155-.027-.153-.029-.151-.03-.15-.03-.148-.031-.146-.032-.145-.033-.143-.034-.141-.035-.14-.035-.137-.037-.136-.037-.134-.038-.132-.038-.13-.04-.128-.04-.126-.041-.124-.042-.122-.042-.12-.044-.117-.043-.116-.045-.113-.045-.112-.046-.109-.047-.106-.047-.105-.048-.102-.049-.1-.049-.097-.05-.095-.05-.093-.052-.09-.051-.087-.052-.085-.053-.083-.054-.08-.054-.077-.054v4.127zm0-5.654v.011l.001.021.003.021.004.021.005.022.006.022.007.022.009.022.01.022.011.023.012.023.013.023.015.024.016.023.017.024.018.024.019.024.021.024.022.024.023.025.024.024.052.05.056.05.061.05.066.051.07.051.075.052.079.051.084.052.088.052.092.052.097.052.102.052.105.052.11.051.114.051.119.052.123.05.127.051.131.05.135.049.139.049.144.048.147.048.152.047.155.046.16.045.163.045.167.044.171.042.176.042.178.04.183.04.187.038.19.037.194.036.197.034.202.033.204.032.209.03.212.028.216.027.219.025.222.024.226.022.23.02.233.018.236.016.24.014.243.012.246.01.249.008.253.006.256.003.259.001.26-.001.257-.003.254-.006.25-.008.247-.01.244-.012.241-.015.237-.016.233-.018.231-.02.226-.022.224-.024.22-.025.216-.027.212-.029.21-.03.205-.032.202-.033.198-.035.194-.036.191-.037.187-.039.183-.039.179-.041.175-.042.172-.043.168-.044.163-.045.16-.045.155-.047.152-.047.148-.048.143-.048.139-.05.136-.049.131-.05.126-.051.123-.051.118-.051.114-.052.11-.052.106-.052.101-.052.096-.052.092-.052.088-.052.083-.052.079-.052.074-.051.07-.052.065-.051.06-.05.056-.051.051-.049.023-.025.023-.024.021-.025.02-.024.019-.024.018-.024.017-.024.015-.023.014-.023.013-.024.012-.022.01-.023.01-.023.008-.022.006-.022.006-.022.004-.021.004-.022.001-.021.001-.021v-4.139l-.077.054-.08.054-.083.054-.085.052-.087.053-.09.051-.093.051-.095.051-.097.05-.1.049-.102.049-.105.048-.106.047-.109.047-.111.046-.114.045-.115.044-.118.044-.12.044-.122.042-.124.042-.126.041-.128.04-.13.039-.132.039-.134.038-.135.037-.138.036-.139.036-.142.035-.143.033-.144.033-.147.033-.148.031-.15.03-.151.03-.153.028-.154.028-.156.027-.158.026-.159.025-.161.024-.162.023-.163.022-.165.021-.166.02-.167.019-.169.018-.169.017-.171.016-.173.015-.173.014-.175.013-.175.012-.177.011-.178.009-.179.009-.179.007-.181.007-.182.005-.182.004-.184.003-.184.002h-.37l-.184-.002-.184-.003-.182-.004-.182-.005-.181-.007-.179-.007-.179-.009-.178-.009-.176-.011-.176-.012-.175-.013-.173-.014-.172-.015-.171-.016-.17-.017-.169-.018-.167-.019-.166-.02-.165-.021-.163-.022-.162-.023-.161-.024-.159-.025-.157-.026-.156-.027-.155-.028-.153-.028-.151-.03-.15-.03-.148-.031-.146-.033-.145-.033-.143-.033-.141-.035-.14-.036-.137-.036-.136-.037-.134-.038-.132-.039-.13-.039-.128-.04-.126-.041-.124-.042-.122-.043-.12-.043-.117-.044-.116-.044-.113-.046-.112-.046-.109-.046-.106-.047-.105-.048-.102-.049-.1-.049-.097-.05-.095-.051-.093-.051-.09-.051-.087-.053-.085-.052-.083-.054-.08-.054-.077-.054v4.139zm0-5.666v.011l.001.02.003.022.004.021.005.022.006.021.007.022.009.023.01.022.011.023.012.023.013.023.015.023.016.024.017.024.018.023.019.024.021.025.022.024.023.024.024.025.052.05.056.05.061.05.066.051.07.051.075.052.079.051.084.052.088.052.092.052.097.052.102.052.105.051.11.052.114.051.119.051.123.051.127.05.131.05.135.05.139.049.144.048.147.048.152.047.155.046.16.045.163.045.167.043.171.043.176.042.178.04.183.04.187.038.19.037.194.036.197.034.202.033.204.032.209.03.212.028.216.027.219.025.222.024.226.021.23.02.233.018.236.017.24.014.243.012.246.01.249.008.253.006.256.003.259.001.26-.001.257-.003.254-.006.25-.008.247-.01.244-.013.241-.014.237-.016.233-.018.231-.02.226-.022.224-.024.22-.025.216-.027.212-.029.21-.03.205-.032.202-.033.198-.035.194-.036.191-.037.187-.039.183-.039.179-.041.175-.042.172-.043.168-.044.163-.045.16-.045.155-.047.152-.047.148-.048.143-.049.139-.049.136-.049.131-.051.126-.05.123-.051.118-.052.114-.051.11-.052.106-.052.101-.052.096-.052.092-.052.088-.052.083-.052.079-.052.074-.052.07-.051.065-.051.06-.051.056-.05.051-.049.023-.025.023-.025.021-.024.02-.024.019-.024.018-.024.017-.024.015-.023.014-.024.013-.023.012-.023.01-.022.01-.023.008-.022.006-.022.006-.022.004-.022.004-.021.001-.021.001-.021v-4.153l-.077.054-.08.054-.083.053-.085.053-.087.053-.09.051-.093.051-.095.051-.097.05-.1.049-.102.048-.105.048-.106.048-.109.046-.111.046-.114.046-.115.044-.118.044-.12.043-.122.043-.124.042-.126.041-.128.04-.13.039-.132.039-.134.038-.135.037-.138.036-.139.036-.142.034-.143.034-.144.033-.147.032-.148.032-.15.03-.151.03-.153.028-.154.028-.156.027-.158.026-.159.024-.161.024-.162.023-.163.023-.165.021-.166.02-.167.019-.169.018-.169.017-.171.016-.173.015-.173.014-.175.013-.175.012-.177.01-.178.01-.179.009-.179.007-.181.006-.182.006-.182.004-.184.003-.184.001-.185.001-.185-.001-.184-.001-.184-.003-.182-.004-.182-.006-.181-.006-.179-.007-.179-.009-.178-.01-.176-.01-.176-.012-.175-.013-.173-.014-.172-.015-.171-.016-.17-.017-.169-.018-.167-.019-.166-.02-.165-.021-.163-.023-.162-.023-.161-.024-.159-.024-.157-.026-.156-.027-.155-.028-.153-.028-.151-.03-.15-.03-.148-.032-.146-.032-.145-.033-.143-.034-.141-.034-.14-.036-.137-.036-.136-.037-.134-.038-.132-.039-.13-.039-.128-.041-.126-.041-.124-.041-.122-.043-.12-.043-.117-.044-.116-.044-.113-.046-.112-.046-.109-.046-.106-.048-.105-.048-.102-.048-.1-.05-.097-.049-.095-.051-.093-.051-.09-.052-.087-.052-.085-.053-.083-.053-.08-.054-.077-.054v4.153zm8.74-8.179l-.257.004-.254.005-.25.008-.247.011-.244.012-.241.014-.237.016-.233.018-.231.021-.226.022-.224.023-.22.026-.216.027-.212.028-.21.031-.205.032-.202.033-.198.034-.194.036-.191.038-.187.038-.183.04-.179.041-.175.042-.172.043-.168.043-.163.045-.16.046-.155.046-.152.048-.148.048-.143.048-.139.049-.136.05-.131.05-.126.051-.123.051-.118.051-.114.052-.11.052-.106.052-.101.052-.096.052-.092.052-.088.052-.083.052-.079.052-.074.051-.07.052-.065.051-.06.05-.056.05-.051.05-.023.025-.023.024-.021.024-.02.025-.019.024-.018.024-.017.023-.015.024-.014.023-.013.023-.012.023-.01.023-.01.022-.008.022-.006.023-.006.021-.004.022-.004.021-.001.021-.001.021.001.021.001.021.004.021.004.022.006.021.006.023.008.022.01.022.01.023.012.023.013.023.014.023.015.024.017.023.018.024.019.024.02.025.021.024.023.024.023.025.051.05.056.05.06.05.065.051.07.052.074.051.079.052.083.052.088.052.092.052.096.052.101.052.106.052.11.052.114.052.118.051.123.051.126.051.131.05.136.05.139.049.143.048.148.048.152.048.155.046.16.046.163.045.168.043.172.043.175.042.179.041.183.04.187.038.191.038.194.036.198.034.202.033.205.032.21.031.212.028.216.027.22.026.224.023.226.022.231.021.233.018.237.016.241.014.244.012.247.011.25.008.254.005.257.004.26.001.26-.001.257-.004.254-.005.25-.008.247-.011.244-.012.241-.014.237-.016.233-.018.231-.021.226-.022.224-.023.22-.026.216-.027.212-.028.21-.031.205-.032.202-.033.198-.034.194-.036.191-.038.187-.038.183-.04.179-.041.175-.042.172-.043.168-.043.163-.045.16-.046.155-.046.152-.048.148-.048.143-.048.139-.049.136-.05.131-.05.126-.051.123-.051.118-.051.114-.052.11-.052.106-.052.101-.052.096-.052.092-.052.088-.052.083-.052.079-.052.074-.051.07-.052.065-.051.06-.05.056-.05.051-.05.023-.025.023-.024.021-.024.02-.025.019-.024.018-.024.017-.023.015-.024.014-.023.013-.023.012-.023.01-.023.01-.022.008-.022.006-.023.006-.021.004-.022.004-.021.001-.021.001-.021-.001-.021-.001-.021-.004-.021-.004-.022-.006-.021-.006-.023-.008-.022-.01-.022-.01-.023-.012-.023-.013-.023-.014-.023-.015-.024-.017-.023-.018-.024-.019-.024-.02-.025-.021-.024-.023-.024-.023-.025-.051-.05-.056-.05-.06-.05-.065-.051-.07-.052-.074-.051-.079-.052-.083-.052-.088-.052-.092-.052-.096-.052-.101-.052-.106-.052-.11-.052-.114-.052-.118-.051-.123-.051-.126-.051-.131-.05-.136-.05-.139-.049-.143-.048-.148-.048-.152-.048-.155-.046-.16-.046-.163-.045-.168-.043-.172-.043-.175-.042-.179-.041-.183-.04-.187-.038-.191-.038-.194-.036-.198-.034-.202-.033-.205-.032-.21-.031-.212-.028-.216-.027-.22-.026-.224-.023-.226-.022-.231-.021-.233-.018-.237-.016-.241-.014-.244-.012-.247-.011-.25-.008-.254-.005-.257-.004-.26-.001-.26.001z" transform="scale(.5)"/></symbol></defs><defs><symbol height="24" width="24" id="clock"><path d="M12 2c5.514 0 10 4.486 10 10s-4.486 10-10 10-10-4.486-10-10 4.486-10 10-10zm0-2c-6.627 0-12 5.373-12 12s5.373 12 12 12 12-5.373 12-12-5.373-12-12-12zm5.848 12.459c.202.038.202.333.001.372-1.907.361-6.045 1.111-6.547 1.111-.719 0-1.301-.582-1.301-1.301 0-.512.77-5.447 1.125-7.445.034-.192.312-.181.343.014l.985 6.238 5.394 1.011z" transform="scale(.5)"/></symbol></defs><defs><marker orient="auto" markerHeight="12" markerWidth="12" markerUnits="userSpaceOnUse" refY="5" refX="7.9" id="arrowhead"><path d="M 0 0 L 10 5 L 0 10 z"/></marker></defs><defs><marker refY="4.5" refX="4" orient="auto" markerHeight="8" markerWidth="15" id="crosshead"><path style="stroke-dasharray: 0, 0;" d="M 1,2 L 6,7 M 6,2 L 1,7" stroke-width="1pt" stroke="#000000" fill="none"/></marker></defs><defs><marker orient="auto" markerHeight="28" markerWidth="20" refY="7" refX="15.5" id="filled-head"><path d="M 18,7 L9,13 L14,7 L9,1 Z"/></marker></defs><defs><marker orient="auto" markerHeight="40" markerWidth="60" refY="15" refX="15" id="sequencenumber"><circle r="6" cy="15" cx="15"/></marker></defs><g><rect class="note" ry="0" rx="0" height="57" width="150" stroke="#666" fill="#EDF2AE" y="75" x="0"/><text style="font-size: 16px; font-weight: 400;" dy="1em" class="noteText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="80" x="75"><tspan x="75">Can be Cron Job or</tspan></text><text style="font-size: 16px; font-weight: 400;" dy="1em" class="noteText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="99" x="75"><tspan x="75">API Request</tspan></text></g><g><rect class="note" ry="0" rx="0" height="76" width="220" stroke="#666" fill="#EDF2AE" y="238" x="550"/><text style="font-size: 16px; font-weight: 400;" dy="1em" class="noteText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="243" x="660"><tspan x="660">1. Endpoint Level (if provided)</tspan></text><text style="font-size: 16px; font-weight: 400;" dy="1em" class="noteText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="262" x="660"><tspan x="660">2. Entity Level</tspan></text><text style="font-size: 16px; font-weight: 400;" dy="1em" class="noteText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="280" x="660"><tspan x="660">3. Tenant Level</tspan></text></g><g><line class="loopLine" y2="468" x2="1071" y1="468" x1="376"/><line class="loopLine" y2="639" x2="1071" y1="468" x1="1071"/><line class="loopLine" y2="639" x2="1071" y1="639" x1="376"/><line class="loopLine" y2="639" x2="376" y1="468" x1="376"/><polygon class="labelBox" points="376,468 426,468 426,481 417.6,488 376,488"/><text style="font-size: 16px; font-weight: 400;" class="labelText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="481" x="401">alt</text><text style="font-size: 16px; font-weight: 400;" class="loopText" text-anchor="middle" y="486" x="748.5"><tspan x="748.5">[Threshold Breached]</tspan></text></g><text style="font-size: 16px; font-weight: 400;" dy="1em" class="messageText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="147" x="266">Check Alert(tenantIDs, envIDs, entity, threshold)</text><line style="fill: none;" marker-end="url(#arrowhead)" stroke="none" stroke-width="2" class="messageLine0" y2="180" x2="456" y1="180" x1="76"/><text style="font-size: 16px; font-weight: 400;" dy="1em" class="messageText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="195" x="559">Get Config Priority</text><line style="fill: none;" marker-end="url(#arrowhead)" stroke="none" stroke-width="2" class="messageLine0" y2="228" x2="656" y1="228" x1="461"/><text style="font-size: 16px; font-weight: 400;" dy="1em" class="messageText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="329" x="562">Effective Config</text><line style="stroke-dasharray: 3, 3; fill: none;" marker-end="url(#arrowhead)" stroke="none" stroke-width="2" class="messageLine1" y2="362" x2="464" y1="362" x1="659"/><text style="font-size: 16px; font-weight: 400;" dy="1em" class="messageText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="377" x="659">Evaluate Entity</text><line style="fill: none;" marker-end="url(#arrowhead)" stroke="none" stroke-width="2" class="messageLine0" y2="410" x2="856" y1="410" x1="461"/><text style="font-size: 16px; font-weight: 400;" dy="1em" class="messageText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="425" x="662">Evaluation Result</text><line style="stroke-dasharray: 3, 3; fill: none;" marker-end="url(#arrowhead)" stroke="none" stroke-width="2" class="messageLine1" y2="458" x2="464" y1="458" x1="859"/><text style="font-size: 16px; font-weight: 400;" dy="1em" class="messageText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="518" x="461">Update Alert State</text><path style="fill: none;" marker-end="url(#arrowhead)" stroke="none" stroke-width="2" class="messageLine0" d="M 461,551 C 521,541 521,581 461,571"/><text style="font-size: 16px; font-weight: 400;" dy="1em" class="messageText" alignment-baseline="middle" dominant-baseline="middle" text-anchor="middle" y="596" x="759">Publish Alert</text><line style="fill: none;" marker-end="url(#arrowhead)" stroke="none" stroke-width="2" class="messageLine0" y2="629" x2="1056" y1="629" x1="461"/></svg>




