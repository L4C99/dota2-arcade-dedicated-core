package engine

import "path/filepath"

// CleanupConfirmedInput is used only after a durable, verified exit result.
// Do not reopen the old PID: it may already belong to an unrelated process.
// File ownership remains checked by the recorded device/inode.
func CleanupConfirmedInput(id Identity) error {
	if id.InputInode == 0 || !filepath.IsAbs(id.RunDirectory) {
		return ErrIdentity
	}
	return removeInput(filepath.Join(id.RunDirectory, "stdin.fifo"), id.InputDevice, id.InputInode)
}
