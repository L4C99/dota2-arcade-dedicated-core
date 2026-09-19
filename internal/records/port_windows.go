package records

import "syscall"

// Windows permits a wildcard bind alongside a specific-address listener unless
// exclusivity is requested. TCP and UDP preflight both require this option.
func exclusiveSocket(network, address string, raw syscall.RawConn) error {
	var optionErr error
	err := raw.Control(func(fd uintptr) { optionErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, -5, 1) })
	if err != nil {
		return err
	}
	return optionErr
}
