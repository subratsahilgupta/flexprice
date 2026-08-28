package main

import (
	"fmt"
	"log"

	"github.com/Shopify/sarama"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/kafka"
	"github.com/flexprice/flexprice/internal/kafka/reconcile"
	"github.com/spf13/cobra"
)

func newKafkaACLsCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "kafka-acls",
		Short: "Seed allow-all Kafka ACLs (MSK SASL/SCRAM lockout safety net)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKafkaACLMigration(dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "log intended ACLs without applying")
	return cmd
}

// seedACLsEnabled gates seeding to the MSK path (SASL/SCRAM). It must NOT key
// off UseSASL: OAUTHBEARER (GCP Managed Kafka) also sets UseSASL=true, and the
// MSK-shaped "kafka-cluster"/User:* ACLs do not apply there.
func seedACLsEnabled(m sarama.SASLMechanism) bool {
	return m == sarama.SASLTypeSCRAMSHA256 || m == sarama.SASLTypeSCRAMSHA512
}

func runKafkaACLMigration(dryRun bool) error {
	cfg, err := config.NewConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if !seedACLsEnabled(cfg.Kafka.SASLMechanism) {
		log.Printf("kafka-acls: SASL mechanism %q is not SCRAM (MSK) — nothing to seed, skipping",
			cfg.Kafka.SASLMechanism)
		return nil
	}

	rules := reconcile.AllowAllACLRules()
	for _, r := range rules {
		log.Printf("desired ACL: %v name=%q principal=%s op=%v",
			r.Resource.ResourceType, r.Resource.ResourceName, r.Acl.Principal, r.Acl.Operation)
	}
	if dryRun {
		log.Printf("kafka-acls: dry-run — %d allow-all rules NOT applied", len(rules))
		return nil
	}

	saramaCfg, err := kafka.GetSaramaConfig(&cfg.Kafka)
	if err != nil {
		return fmt.Errorf("build kafka client config: %w", err)
	}
	admin, err := sarama.NewClusterAdmin(cfg.Kafka.Brokers, saramaCfg)
	if err != nil {
		return fmt.Errorf("connect cluster admin: %w", err)
	}
	defer admin.Close()

	sa := &reconcile.SaramaAdmin{Admin: admin}
	for _, r := range rules {
		// CreateACL on an existing binding is a broker no-op → idempotent.
		if err := sa.CreateACL(r.Resource, r.Acl); err != nil {
			return fmt.Errorf("create ACL %v/%q: %w", r.Resource.ResourceType, r.Resource.ResourceName, err)
		}
		log.Printf("OK seeded ACL %v name=%q", r.Resource.ResourceType, r.Resource.ResourceName)
	}
	log.Printf("kafka-acls done: %d allow-all rules ensured", len(rules))
	return nil
}
