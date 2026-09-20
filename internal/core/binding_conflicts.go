package core

import (
	"fmt"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
	"net"
	"sort"
	"strings"
)

func overlap(a, b engine.Binding) bool {
	if a.Protocol != b.Protocol || a.Port != b.Port {
		return false
	}
	x, y := net.ParseIP(a.Address), net.ParseIP(b.Address)
	return x != nil && y != nil && (x.IsUnspecified() || y.IsUnspecified() || x.Equal(y))
}
func serviceBinding(b engine.Binding, all []engine.Binding, main int) bool {
	if strings.HasPrefix(b.Protocol, "tcp") || b.Port == main {
		return true
	}
	if !strings.HasPrefix(b.Protocol, "udp") {
		return false
	}
	tcp := b
	tcp.Protocol = "tcp" + strings.TrimPrefix(b.Protocol, "udp")
	for _, other := range all {
		if overlap(tcp, other) {
			return true
		}
	}
	return false
}

// Called under the manager lock; other process identities are revalidated, not
// inferred from their old persisted bindings. No socket is closed or reassigned.
func (m *Manager) checkBindingConflicts(in *records.Instance, bindings []engine.Binding) *records.PortCheck {
	result := &records.PortCheck{Status: "complete", Notes: []string{}, Conflicts: []string{}}
	partial := func(note string) { result.Notes = append(result.Notes, note) }
	services := []engine.Binding{}
	for _, b := range bindings {
		if serviceBinding(b, bindings, in.Port) {
			services = append(services, b)
		} else {
			partial(fmt.Sprintf("unclassified UDP %s %s:%d", b.Protocol, b.Address, b.Port))
		}
		if b.Address == "::" {
			partial("IPv6 wildcard dual-stack coverage is unverified")
		}
	}
	ids := []string{}
	for id, peer := range m.store.State.Instances {
		if id != in.ID && peer.Lifecycle != "reclaimed" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		peer := m.store.State.Instances[id]
		for _, b := range services {
			if b.Port == peer.Port {
				result.Conflicts = append(result.Conflicts, fmt.Sprintf("%s %s:%d overlaps %s game reservation", b.Protocol, b.Address, b.Port, id))
			}
		}
		if peer.Process == "stopped" {
			continue
		}
		if len(peer.Runs) == 0 || peer.Runs[len(peer.Runs)-1].Identity == nil {
			partial("peer identity unverified: " + id)
			continue
		}
		h, e := engine.Open(*peer.Runs[len(peer.Runs)-1].Identity)
		if e != nil {
			partial("peer identity unavailable: " + id)
			continue
		}
		read := m.bindings
		if read == nil {
			read = (*engine.Handle).Bindings
		}
		other, e := read(h)
		alive, ae := h.Alive()
		h.Close()
		if e != nil || ae != nil || !alive {
			partial("peer bindings unavailable: " + id)
			continue
		}
		for _, b := range other {
			if !serviceBinding(b, other, peer.Port) {
				partial(fmt.Sprintf("peer %s unclassified UDP %s %s:%d", id, b.Protocol, b.Address, b.Port))
				continue
			}
			for _, a := range services {
				if overlap(a, b) {
					result.Conflicts = append(result.Conflicts, fmt.Sprintf("%s %s:%d overlaps %s %s:%d", a.Protocol, a.Address, a.Port, id, b.Address, b.Port))
				}
				if a.Port == b.Port && a.Protocol != b.Protocol && strings.TrimRight(a.Protocol, "46") == strings.TrimRight(b.Protocol, "46") && (a.Address == "::" || b.Address == "::") {
					partial("cross-family wildcard overlap is unverified: " + id)
				}
			}
		}
	}
	sort.Strings(result.Conflicts)
	sort.Strings(result.Notes)
	if len(result.Notes) > 0 {
		result.Status = "partial"
	}
	if len(result.Conflicts) > 0 {
		result.Status = "conflict"
	}
	return result
}
