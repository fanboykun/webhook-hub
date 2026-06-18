package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/fanboykun/webhook-hub/internal/domain"
)

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func validateIngestBatch(batch domain.IngestBatch) error {
	if batch.Receipt.ID == "" {
		return fmt.Errorf("receipt id is required")
	}
	if batch.Receipt.Source == "" {
		return fmt.Errorf("receipt source is required")
	}
	for _, event := range batch.Events {
		if event.ID == "" {
			return fmt.Errorf("event id is required")
		}
		if event.ReceiptID != "" && event.ReceiptID != batch.Receipt.ID {
			return fmt.Errorf("event %q receipt_id mismatch", event.ID)
		}
		for _, delivery := range batch.DeliveryByEvent[event.ID] {
			if delivery.ID == "" {
				return fmt.Errorf("delivery id is required")
			}
			if delivery.EventID != event.ID {
				return fmt.Errorf("delivery %q event_id mismatch", delivery.ID)
			}
		}
	}
	return nil
}
