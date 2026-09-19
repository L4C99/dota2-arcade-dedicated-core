// Package localipc provides a same-user, local-only, bounded request transport.
package localipc

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
)

const MaxRequest = 1 << 20
const MaxResponse = 2 << 20
const ioTimeout = 5 * time.Second

var ErrLocked = errors.New("another manager owns this data directory")
var ErrVersion = errors.New("unsupported endpoint protocol version")

type Endpoint struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Type            string `json:"type"`
	Address         string `json:"address"`
}
type Server struct {
	listener      net.Listener
	release       func() error
	cleanup       func() error
	endpointPath  string
	endpointBytes []byte
	once          sync.Once
	closeErr      error
}

func Listen(dataDir string) (*Server, error) {
	if !filepath.IsAbs(dataDir) {
		return nil, fmt.Errorf("absolute data directory required")
	}
	if e := prepareDirectory(filepath.Clean(dataDir)); e != nil {
		return nil, e
	}
	manager := filepath.Join(filepath.Clean(dataDir), "manager")
	if e := prepareDirectory(manager); e != nil {
		return nil, e
	}
	release, e := acquireLock(filepath.Join(manager, "lock"))
	if e != nil {
		return nil, e
	}
	var token [16]byte
	if _, e = rand.Read(token[:]); e != nil {
		release()
		return nil, e
	}
	listener, ep, cleanup, e := listenPlatform(manager, hex.EncodeToString(token[:]))
	if e != nil {
		release()
		return nil, e
	}
	s := &Server{listener: listener, release: release, cleanup: cleanup, endpointPath: filepath.Join(manager, "endpoint.json")}
	s.endpointBytes, e = json.Marshal(ep)
	if e != nil {
		s.Close()
		return nil, e
	}
	// Never follow an endpoint symlink or overwrite unrelated non-regular data.
	if st, err := os.Lstat(s.endpointPath); err == nil && !st.Mode().IsRegular() {
		s.Close()
		return nil, fmt.Errorf("endpoint is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.Close()
		return nil, err
	}
	if prior, e := os.ReadFile(s.endpointPath); e == nil {
		var old Endpoint
		if len(prior) > 4096 || config.DecodeStrict(prior, &old) != nil || old.ProtocolVersion != 1 || old.Type != ep.Type || !ownedEndpoint(manager, old) {
			s.Close()
			return nil, fmt.Errorf("existing endpoint is not a recognized manager record; refusing overwrite")
		}
	}
	tmp, e := os.CreateTemp(manager, ".endpoint-*")
	if e != nil {
		s.Close()
		return nil, e
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, e = tmp.Write(s.endpointBytes); e == nil {
		e = tmp.Sync()
	}
	closeErr := tmp.Close()
	if e == nil {
		e = closeErr
	}
	if e == nil {
		e = os.Rename(tmpPath, s.endpointPath)
	}
	if e != nil {
		s.Close()
		return nil, e
	}
	return s, nil
}

func ownedEndpoint(manager string, ep Endpoint) bool {
	switch ep.Type {
	case "unix":
		return filepath.Dir(ep.Address) == manager && strings.HasPrefix(filepath.Base(ep.Address), "ipc-") && strings.HasSuffix(ep.Address, ".sock")
	case "named-pipe":
		prefix := `\\.\pipe\d2core-`
		if !strings.HasPrefix(ep.Address, prefix) {
			return false
		}
		tail := strings.TrimPrefix(ep.Address, prefix)
		_, err := hex.DecodeString(tail)
		return err == nil && len(tail) == 32
	default:
		return false
	}
}
func (s *Server) Close() error {
	s.once.Do(func() {
		e := s.listener.Close()
		if errors.Is(e, net.ErrClosed) {
			e = nil
		}
		if b, readErr := os.ReadFile(s.endpointPath); readErr == nil && bytes.Equal(b, s.endpointBytes) {
			e = errors.Join(e, os.Remove(s.endpointPath))
		}
		e = errors.Join(e, s.cleanup())
		e = errors.Join(e, s.release())
		s.closeErr = e
	})
	return s.closeErr
}
func (s *Server) Serve(ctx context.Context, handler func([]byte) []byte) error {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			// Stop accepting without releasing the writer lock. The caller must
			// drain accepted manager work before calling Server.Close.
			s.listener.Close()
		case <-done:
		}
	}()
	slots := make(chan struct{}, 32)
	for {
		conn, e := s.listener.Accept()
		if e != nil {
			if ctx.Err() != nil || errors.Is(e, net.ErrClosed) {
				return nil
			}
			return e
		}
		select {
		case slots <- struct{}{}:
			go func() {
				defer func() { <-slots }()
				defer conn.Close()
				if e := authorizePeer(conn); e != nil {
					return
				}
				_ = conn.SetDeadline(time.Now().Add(ioTimeout))
				line, e := readLine(conn, MaxRequest)
				if e != nil {
					return
				}
				response := handler(line)
				if len(response) > MaxResponse || bytes.ContainsAny(response, "\r\n") {
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(ioTimeout))
				_, _ = conn.Write(append(response, '\n'))
			}()
		default:
			conn.Close()
		}
	}
}
func readLine(r io.Reader, max int) ([]byte, error) {
	br := bufio.NewReader(io.LimitReader(r, int64(max)+2))
	line, e := br.ReadBytes('\n')
	if e != nil {
		return nil, e
	}
	if len(line) > max+1 {
		return nil, fmt.Errorf("message exceeds size limit")
	}
	line = line[:len(line)-1]
	if bytes.ContainsRune(line, '\r') {
		return nil, fmt.Errorf("carriage returns are not allowed")
	}
	return line, nil
}
func Call(ctx context.Context, dataDir string, request []byte) ([]byte, error) {
	if !filepath.IsAbs(dataDir) {
		return nil, fmt.Errorf("absolute data directory required")
	}
	if len(request) > MaxRequest || bytes.ContainsAny(request, "\r\n") {
		return nil, fmt.Errorf("request must be one bounded JSON line")
	}
	if e := verifyDirectory(filepath.Clean(dataDir)); e != nil {
		return nil, e
	}
	manager := filepath.Join(filepath.Clean(dataDir), "manager")
	if e := verifyDirectory(manager); e != nil {
		return nil, e
	}
	path := filepath.Join(manager, "endpoint.json")
	st, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > 4096 {
		return nil, fmt.Errorf("invalid endpoint file")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var ep Endpoint
	if e = config.DecodeStrict(b, &ep); e != nil {
		return nil, e
	}
	if ep.ProtocolVersion != 1 {
		return nil, ErrVersion
	}
	callCtx, cancel := context.WithTimeout(ctx, ioTimeout)
	defer cancel()
	conn, e := dialPlatform(callCtx, manager, ep)
	if e != nil {
		return nil, e
	}
	defer conn.Close()
	deadline, _ := callCtx.Deadline()
	_ = conn.SetDeadline(deadline)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-callCtx.Done():
			conn.Close()
		case <-done:
		}
	}()
	if _, e = conn.Write(append(append([]byte(nil), request...), '\n')); e != nil {
		return nil, e
	}
	return readLine(conn, MaxResponse)
}
