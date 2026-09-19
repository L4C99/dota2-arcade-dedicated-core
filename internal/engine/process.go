// Package engine implements identity-checked native process control. It never
// uses PID alone to signal a process and never attaches lifetime to the manager.
package engine

import (
	"errors"
)

var ErrGone = errors.New("recorded process has exited")
var ErrIdentity = errors.New("process identity could not be verified")

type Spec struct {
	Executable       string   `json:"executable"`
	WorkingDirectory string   `json:"workingDirectory"`
	Arguments        []string `json:"arguments"`
	RunDirectory     string   `json:"runDirectory"`
}

// Identity is persisted before control operations. Platform fields are exact
// native values, not rounded wall-clock timestamps. Zero identities fail closed.
type Identity struct {
	PID          int      `json:"pid"`
	Executable   string   `json:"executable"`
	CreationTime uint64   `json:"creationTime,omitempty"`
	BootID       string   `json:"bootId,omitempty"`
	StartTicks   uint64   `json:"startTicks,omitempty"`
	InputDevice  uint64   `json:"inputDevice,omitempty"`
	InputInode   uint64   `json:"inputInode,omitempty"`
	Arguments    []string `json:"arguments"`
	RunDirectory string   `json:"runDirectory"`
}

type Binding struct {
	Protocol   string `json:"protocol"`
	Address    string `json:"address"`
	Port       int    `json:"port"`
	PID        int    `json:"pid"`
	ObservedAt string `json:"observedAt"`
}
