package records

import (
	"fmt"
	"net"
	"os"
	"testing"
)

func TestAutomaticReservationRetryAndExhaustion(t *testing.T) {
	s, path := setup(t)
	p := availablePort(t)
	r := PortRange{p, p}
	in, op, err := s.CreateInRange(path, 0, "auto", r)
	if err != nil || in.Port != p {
		t.Fatalf("allocation: %v %v", in, err)
	}
	_, _, err = s.CreateInRange(path, 0, "other", r)
	requireCode(t, err, "NO_PORT_AVAILABLE")
	_, _, err = s.CreateInRange(path, p, "auto", r)
	requireCode(t, err, "IDEMPOTENCY_CONFLICT")
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	again, same, err := reloaded.CreateInRange(path, 0, "auto", PortRange{1, 1})
	if err != nil || again.ID != in.ID || same.ID != op.ID || again.Port != p {
		t.Fatalf("retry changed reservation: %v", err)
	}
}

func TestAutomaticProbeRejectsExternalUDPAndInvalidRanges(t *testing.T) {
	s, path := setup(t)
	p := availablePort(t)
	l, err := net.ListenPacket("udp4", fmt.Sprintf("127.0.0.1:%d", p))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, _, err = s.CreateInRange(path, 0, "blocked", PortRange{p, p})
	requireCode(t, err, "NO_PORT_AVAILABLE")
	if len(s.State.Keys) != 0 || len(s.State.Instances) != 0 {
		t.Fatal("exhaustion wrote intent")
	}
	for _, r := range []PortRange{{0, 1}, {2, 1}, {1, 65536}, {1, 4097}} {
		if r.Validate() == nil {
			t.Fatalf("invalid range accepted: %v", r)
		}
	}
}
