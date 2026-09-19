package core

import (
	"errors"
	"fmt"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
)

type Options struct {
	Ports       records.PortRange
	HistoryDays int
	MinFreeMiB  int
}

func DefaultOptions() Options { return Options{records.DefaultPortRange(), 7, 1024} }
func (o Options) Validate() error {
	if err := o.Ports.Validate(); err != nil {
		return err
	}
	if o.HistoryDays < 1 || o.HistoryDays > 3650 || o.MinFreeMiB < 1 || o.MinFreeMiB > 1048576 {
		return fmt.Errorf("history days must be 1..3650 and minimum free MiB 1..1048576")
	}
	return nil
}

type storageStatus struct {
	CheckedAt    time.Time        `json:"checkedAt"`
	FreeBytes    uint64           `json:"freeBytes"`
	MinFreeBytes uint64           `json:"minFreeBytes"`
	HistoryDays  int              `json:"historyDays"`
	Error        *records.Failure `json:"error"`
}

func (m *Manager) checkSpace(path string) *records.Failure {
	free, err := m.freeBytes(path)
	if err != nil {
		return fail("IO_ERROR", "storage", err.Error())
	}
	if free < uint64(m.options.MinFreeMiB)<<20 {
		return fail("INSUFFICIENT_STORAGE", "storage", "available disk space below configured reserve: "+path)
	}
	return nil
}

func (m *Manager) maintainStorage(now time.Time) {
	if m.writeError != nil {
		return
	}
	status := storageStatus{CheckedAt: now, MinFreeBytes: uint64(m.options.MinFreeMiB) << 20, HistoryDays: m.options.HistoryDays}
	_, err := m.store.PruneExpired(now, time.Duration(m.options.HistoryDays)*24*time.Hour, 16)
	if err != nil {
		var f *records.Failure
		if errors.As(err, &f) {
			status.Error = f
			if f.Stage == "persist" {
				m.writeError = err
			}
		} else {
			status.Error = classify(err, "cleanup")
		}
	}
	var spaceErr error
	status.FreeBytes, spaceErr = m.freeBytes(m.store.Dir)
	if status.Error == nil {
		if spaceErr != nil {
			status.Error = fail("IO_ERROR", "storage", spaceErr.Error())
		} else if status.FreeBytes < status.MinFreeBytes {
			status.Error = fail("INSUFFICIENT_STORAGE", "storage", "available disk space below configured reserve")
		}
	}
	m.storage = status
}
