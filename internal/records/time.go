package records

import "time"

// OrderedTime keeps persisted ordering valid across wall-clock corrections.
// It does not change deadlines, process identities, or corruption validation.
func OrderedTime(now time.Time, floors ...time.Time) time.Time {
	for _, floor := range floors {
		if now.Before(floor) {
			now = floor
		}
	}
	return now.UTC()
}
