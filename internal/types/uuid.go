package types

import (
	"fmt"
	"strings"
	"sync"

	"github.com/oklog/ulid/v2"
	"github.com/teris-io/shortid"
)

// GenerateUUID returns a k-sortable unique identifier
func GenerateUUID() string {
	return ulid.Make().String()
}

// GenerateUUIDWithPrefix returns a k-sortable unique identifier
// with a prefix ex inv_0ujsswThIGTUYm2K8FjOOfXtY1K
func GenerateUUIDWithPrefix(prefix string) string {
	if prefix == "" {
		return GenerateUUID()
	}
	return fmt.Sprintf("%s_%s", prefix, GenerateUUID())
}

var (
	sidGenerator *shortid.Shortid
	once         sync.Once
)

// initializeSID initializes the shortid generator once
func initializeSID() {
	var err error
	sidGenerator, err = shortid.New(1, shortid.DefaultABC, 2342)
	if err != nil {
		panic("failed to initialize shortid generator: " + err.Error())
	}
}

// GenerateShortIDWithPrefix returns a short ID with a prefix.
// Total length is capped at 12 characters, e.g., `in_xYZ12A8Q`.
func GenerateShortIDWithPrefix(prefix string) string {
	once.Do(initializeSID)

	id, err := sidGenerator.Generate()
	if err != nil {
		return ""
	}
	id = strings.ReplaceAll(id, "-", "")

	availableLen := 12 - len(prefix)
	if availableLen <= 0 {
		return ""
	}

	if len(id) > availableLen {
		id = id[:availableLen]
	}

	shortId := strings.ToUpper(fmt.Sprintf("%s%s", prefix, id))

	return shortId
}

const (
	// Prefixes for all domains and entities

	UUID_PREFIX_EVENT                       = "event"
	UUID_PREFIX_METER                       = "meter"
	UUID_PREFIX_PLAN                        = "plan"
	UUID_PREFIX_PRICE                       = "price"
	UUID_PREFIX_INVOICE                     = "inv"
	UUID_PREFIX_INVOICE_LINE_ITEM           = "inv_line"
	UUID_PREFIX_SUBSCRIPTION                = "subs"
	UUID_PREFIX_SUBSCRIPTION_LINE_ITEM      = "subs_line"
	UUID_PREFIX_SUBSCRIPTION_PAUSE          = "pause"
	UUID_PREFIX_SUBSCRIPTION_SCHEDULE       = "sched"
	UUID_PREFIX_SUBSCRIPTION_SCHEDULE_PHASE = "phase"
	UUID_PREFIX_CUSTOMER                    = "cust"
	UUID_PREFIX_WALLET                      = "wallet"
	UUID_PREFIX_WALLET_TRANSACTION          = "wtxn"
	UUID_PREFIX_ENVIRONMENT                 = "env"
	UUID_PREFIX_USER                        = "user"
	UUID_PREFIX_TENANT                      = "tenant"
	UUID_PREFIX_FEATURE                     = "feat"
	UUID_PREFIX_ENTITLEMENT                 = "ent"
	UUID_PREFIX_PAYMENT                     = "pay"
	UUID_PREFIX_PAYMENT_ATTEMPT             = "attempt"
	UUID_PREFIX_TASK                        = "task"
	UUID_PREFIX_SECRET                      = "secret"
	UUID_PREFIX_CREDIT_GRANT                = "cg"
	UUID_PREFIX_COSTSHEET                   = "cost"
	UUID_PREFIX_CREDIT_GRANT_APPLICATION    = "cga"
	UUID_PREFIX_CREDIT_NOTE                 = "cn"
	UUID_PREFIX_CREDIT_NOTE_LINE_ITEM       = "cn_line"
	UUID_PREFIX_COUPON                      = "coupon"
	UUID_PREFIX_COUPON_ASSOCIATION          = "coupon_assoc"
	UUID_PREFIX_COUPON_APPLICATION          = "coupon_app"
	UUID_PREFIX_PRICE_UNIT                  = "price_unit"

	UUID_PREFIX_WEBHOOK_EVENT = "webhook"
)

const (
	SHORT_ID_PREFIX_CREDIT_NOTE = "CN-"
)
