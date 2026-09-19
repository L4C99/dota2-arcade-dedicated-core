package records

import "fmt"

// PortRange controls only new automatic allocations, never existing instances.
type PortRange struct{ Min, Max int }

func DefaultPortRange() PortRange { return PortRange{27015, 27064} }

func (r PortRange) Validate() error {
	if r.Min < 1 || r.Max > 65535 || r.Min > r.Max || r.Max-r.Min >= 4096 {
		return fmt.Errorf("port range must contain 1..4096 ports within 1..65535")
	}
	return nil
}

func (s *Store) portReserved(port int) bool {
	for _, in := range s.State.Instances {
		if in.Lifecycle != "reclaimed" && in.Port == port {
			return true
		}
	}
	return false
}

// Called under the manager lock. OS probes are not persistent socket leases.
func (s *Store) allocatePort(requested int, ports PortRange) (int, error) {
	if requested != 0 {
		if s.portReserved(requested) {
			return 0, &Failure{Code: "PORT_IN_USE", Stage: "validate", Message: "port reserved by an active or unreclaimed instance"}
		}
		if err := CheckPort(requested); err != nil {
			return 0, err
		}
		return requested, nil
	}
	if err := ports.Validate(); err != nil {
		return 0, err
	}
	for port := ports.Min; port <= ports.Max; port++ {
		if !s.portReserved(port) && CheckPort(port) == nil {
			return port, nil
		}
	}
	return 0, &Failure{Code: "NO_PORT_AVAILABLE", Stage: "validate", Message: "no usable unreserved TCP/UDP port in configured range"}
}
