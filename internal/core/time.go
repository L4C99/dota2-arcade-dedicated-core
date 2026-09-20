package core

import (
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
	"time"
)

// Called under the manager mutex. Clock injection never changes the host clock.
func (m *Manager) recordTime(in *records.Instance, op *records.Operation) time.Time {
	now := time.Now()
	if m.clock != nil {
		now = m.clock()
	}
	floors := []time.Time{in.CreatedAt, in.UpdatedAt}
	if op != nil {
		floors = append(floors, op.CreatedAt)
		if op.FinishedAt != nil {
			floors = append(floors, *op.FinishedAt)
		}
	}
	return records.OrderedTime(now, floors...)
}
