package core

import "github.com/L4C99/dota2-arcade-dedicated-core/internal/records"

func sameFailure(a, b *records.Failure) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Code == b.Code && a.Stage == b.Stage && a.Message == b.Message && a.InstanceID == b.InstanceID && a.OperationID == b.OperationID
}
