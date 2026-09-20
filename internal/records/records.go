// Package records owns instance snapshots and generation files. The manager is
// the sole writer; callers serialize mutations under its data-directory lock.
package records

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
)

const FormatVersion = 2
const MaxStateBytes = 64 << 20

type Failure struct {
	cause       error
	Code        string `json:"code"`
	Stage       string `json:"stage"`
	Message     string `json:"message"`
	InstanceID  string `json:"instanceId,omitempty"`
	OperationID string `json:"operationId,omitempty"`
}

func (e *Failure) Error() string { return e.Code + ": " + e.Message }
func (e *Failure) Unwrap() error { return e.cause }

type Run struct {
	SpawnAttempted  bool               `json:"spawnAttempted"`
	StartupDeadline time.Time          `json:"startupDeadline"`
	Generation      int                `json:"generation"`
	Directory       string             `json:"directory"`
	LogPath         string             `json:"logPath"`
	CFGPath         string             `json:"cfgPath"`
	CFGDigest       string             `json:"cfgDigest"`
	Arguments       []string           `json:"arguments"`
	CreatedAt       time.Time          `json:"createdAt"`
	Prepared        bool               `json:"prepared"`
	CFGCreated      bool               `json:"cfgCreated"`
	CFGRemoved      bool               `json:"cfgRemoved"`
	InputRemoved    bool               `json:"inputRemoved"`
	Identity        *engine.Identity   `json:"identity"`
	Evidence        *engine.Evidence   `json:"evidence"`
	Bindings        []engine.Binding   `json:"bindings"`
	StopResult      *engine.StopResult `json:"stopResult"`
}
type Instance struct {
	ID                 string          `json:"instanceId"`
	TemplatePath       string          `json:"templatePath"`
	Snapshot           config.Template `json:"snapshot"`
	Port               int             `json:"port"`
	Generation         int             `json:"generation"`
	Lifecycle          string          `json:"lifecycle"`
	Process            string          `json:"process"`
	Room               string          `json:"room"`
	Cleanup            string          `json:"cleanup"`
	CurrentOperationID string          `json:"currentOperationId"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
	Runs               []*Run          `json:"runs"`
	Error              *Failure        `json:"error"`
}
type Operation struct {
	Phase      string     `json:"phase"`
	ID         string     `json:"operationId"`
	Kind       string     `json:"kind"`
	InstanceID string     `json:"instanceId"`
	Generation int        `json:"generation"`
	Status     string     `json:"status"`
	Error      *Failure   `json:"error"`
	CreatedAt  time.Time  `json:"createdAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}
type Key struct {
	Fingerprint string `json:"fingerprint"`
	InstanceID  string `json:"instanceId"`
	OperationID string `json:"operationId"`
}
type State struct {
	Checksum      string                `json:"checksum"`
	FormatVersion int                   `json:"formatVersion"`
	Instances     map[string]*Instance  `json:"instances"`
	Operations    map[string]*Operation `json:"operations"`
	Keys          map[string]Key        `json:"keys"`
}
type Store struct {
	Dir      string
	State    State
	saveHook func(string) error // Tests inject failures; nil in production.
}

func ID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(b[:]), nil
}
func validID(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix+"_") || len(s) != len(prefix)+33 {
		return false
	}
	_, err := hex.DecodeString(s[len(prefix)+1:])
	return err == nil
}
func digest(b []byte) string { d := sha256.Sum256(b); return hex.EncodeToString(d[:]) }

// Open must be called only after acquiring the manager's exclusive lock.
func Open(dir string) (*Store, error) {
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("absolute data directory required")
	}
	dir = filepath.Clean(dir)
	for _, r := range dir {
		if r < 32 || r > 126 {
			return nil, fmt.Errorf("data directory must use ASCII for engine logs")
		}
	}
	if err := durableMkdirAll(dir); err != nil {
		return nil, err
	}
	s := &Store{Dir: dir, State: State{FormatVersion: FormatVersion, Instances: map[string]*Instance{}, Operations: map[string]*Operation{}, Keys: map[string]Key{}}}
	markerExists, err := verifyMarker(dir)
	if err != nil {
		return nil, err
	}
	statePath := filepath.Join(dir, "state.json")
	st, err := os.Lstat(statePath)
	if errors.Is(err, os.ErrNotExist) {
		if markerExists {
			return nil, fmt.Errorf("initialized store has no state.json; refusing empty recovery")
		}
		if _, entryErr := os.Lstat(filepath.Join(dir, "instances")); entryErr == nil || !os.IsNotExist(entryErr) {
			return nil, fmt.Errorf("instance files exist without state; refusing empty recovery")
		}
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("state must be a regular file")
	}
	if !markerExists {
		return nil, fmt.Errorf("state exists without initialization marker; refusing damaged store")
	}
	f, err := os.Open(statePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxStateBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxStateBytes {
		return nil, fmt.Errorf("state exceeds 64 MiB; refusing partial load")
	}
	var loaded State
	if err = config.DecodeStrict(b, &loaded); err != nil {
		return nil, fmt.Errorf("invalid state: %w", err)
	}
	s.State = loaded
	if s.State.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("unsupported data formatVersion")
	}
	expected, err := stateChecksum(s.State)
	if err != nil {
		return nil, err
	}
	if s.State.Checksum != expected {
		return nil, fmt.Errorf("state checksum mismatch")
	}
	if err = s.validateState(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Save() error {
	if err := s.validateState(); err != nil {
		return err
	}
	next := s.State
	checksum, err := stateChecksum(next)
	if err != nil {
		return err
	}
	next.Checksum = checksum
	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > MaxStateBytes {
		return fmt.Errorf("state exceeds 64 MiB; refusing save")
	}
	if err = ensureInitialized(s.Dir); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.Dir, ".state-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(b); err == nil {
		if s.saveHook != nil {
			err = s.saveHook("temp-written")
		}
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if s.saveHook != nil {
		if err = s.saveHook("before-replace"); err != nil {
			return err
		}
	}
	published, err := replaceState(tmp, filepath.Join(s.Dir, "state.json"))
	if published {
		s.State.Checksum = checksum
	}
	if err == nil && s.saveHook != nil {
		err = s.saveHook("after-replace")
	}
	if err != nil && published {
		return &DurabilityError{Err: err}
	}
	return err
}

// CheckPort probes both transport families. This is a preflight, not a lease.
func CheckPort(port int) error {
	if port < 1 || port > 65535 {
		return &Failure{Code: "PORT_REQUIRED", Stage: "validate", Message: "explicit port 1..65535 required"}
	}
	var opened []io.Closer
	lc := net.ListenConfig{Control: exclusiveSocket}
	defer func() {
		for _, c := range opened {
			c.Close()
		}
	}()
	for _, network := range []string{"tcp4", "tcp6", "udp4", "udp6"} {
		address := fmt.Sprintf(":%d", port)
		var c io.Closer
		var err error
		if strings.HasPrefix(network, "tcp") {
			c, err = lc.Listen(context.Background(), network, address)
		} else {
			c, err = lc.ListenPacket(context.Background(), network, address)
		}
		if err != nil {
			return &Failure{Code: "PORT_IN_USE", Stage: "validate", Message: network + ": " + err.Error()}
		}
		opened = append(opened, c)
	}
	return nil
}

func Fingerprint(path string, port int) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute template path required")
	}
	b, _ := json.Marshal(struct {
		Path string
		Port int
	}{filepath.Clean(path), port})
	return digest(b), nil
}
func ValidateKey(key string) error {
	if len(key) < 1 || len(key) > 128 {
		return fmt.Errorf("idempotency key must contain 1..128 ASCII characters")
	}
	for _, r := range key {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return fmt.Errorf("invalid idempotency key")
		}
	}
	return nil
}

// Create persists the fully normalized snapshot and reservation before any run
// files or processes exist. Retry lookup happens before rereading the template.
func (s *Store) Create(path string, port int, key string) (*Instance, *Operation, error) {
	return s.CreateInRange(path, port, key, DefaultPortRange())
}

func (s *Store) CreateInRange(path string, port int, key string, ports PortRange) (*Instance, *Operation, error) {
	if err := ValidateKey(key); err != nil {
		return nil, nil, &Failure{Code: "INVALID_REQUEST", Stage: "validate", Message: err.Error()}
	}
	fp, err := Fingerprint(path, port)
	if err != nil {
		return nil, nil, err
	}
	if old, ok := s.State.Keys[key]; ok {
		if old.Fingerprint != fp {
			return nil, nil, &Failure{Code: "IDEMPOTENCY_CONFLICT", Stage: "validate", Message: "key already used with different parameters"}
		}
		return s.State.Instances[old.InstanceID], s.State.Operations[old.OperationID], nil
	}
	t, err := config.Load(path)
	if err != nil {
		return nil, nil, err
	}
	// Fingerprint retains requested port 0 for automatic intent; the selected
	// port is saved on the instance and never reselected on a retry.
	port, err = s.allocatePort(port, ports)
	if err != nil {
		return nil, nil, err
	}
	id, err := ID("i")
	if err != nil {
		return nil, nil, err
	}
	oid, err := ID("o")
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	in := &Instance{ID: id, TemplatePath: filepath.Clean(path), Snapshot: t, Port: port, Lifecycle: "active", Process: "stopped", Room: "unknown", Cleanup: "pending", CurrentOperationID: oid, CreatedAt: now, UpdatedAt: now, Runs: []*Run{}}
	op := &Operation{ID: oid, Kind: "create", InstanceID: id, Status: "running", Phase: "accepted", CreatedAt: now}
	s.State.Instances[id] = in
	s.State.Operations[oid] = op
	s.State.Keys[key] = Key{fp, id, oid}
	if err = s.Save(); err != nil {
		var uncertain *DurabilityError
		if errors.As(err, &uncertain) {
			return in, op, &Failure{Code: "IO_ERROR", Stage: "persist", Message: err.Error(), InstanceID: id, OperationID: oid, cause: err}
		}
		delete(s.State.Instances, id)
		delete(s.State.Operations, oid)
		delete(s.State.Keys, key)
		return nil, nil, err
	}
	return in, op, nil
}

// PrepareRun writes an exclusive cfg and separate archived copy. A failed
// preparation keeps the intent and ownership metadata for explicit cleanup.
func (s *Store) PrepareRun(in *Instance) (*Run, error) {
	if !validID(in.ID, "i") || s.State.Instances[in.ID] != in {
		return nil, fmt.Errorf("unowned instance")
	}
	if in.Process != "stopped" || in.Lifecycle != "active" {
		return nil, fmt.Errorf("cannot prepare run in current state")
	}
	for _, previous := range in.Runs {
		if !previous.CFGRemoved {
			return nil, fmt.Errorf("previous generated cfg must be cleaned before another generation")
		}
	}
	g := in.Generation + 1
	dir := filepath.Join(s.Dir, "instances", in.ID, "runs", fmt.Sprintf("%06d", g))
	name := fmt.Sprintf("d2core_%s_g%d.cfg", in.ID, g)
	r := &Run{Generation: g, Directory: dir, LogPath: filepath.Join(dir, "engine.log"), CFGPath: filepath.Join(in.Snapshot.CFG.Directory, name), CreatedAt: time.Now().UTC()}
	expanded, err := config.Expand(in.Snapshot, config.Values{InstanceID: in.ID, CfgName: name, LogPath: r.LogPath, GamePort: in.Port})
	if err != nil {
		return nil, err
	}
	r.Arguments = expanded.Arguments
	r.CFGDigest = digest([]byte(expanded.CFG))
	in.Generation = g
	in.Runs = append(in.Runs, r)
	in.UpdatedAt = OrderedTime(time.Now(), in.CreatedAt, in.UpdatedAt)
	if err = s.Save(); err != nil {
		return r, err
	}
	if err = durableMkdirAll(filepath.Dir(dir)); err != nil {
		return r, err
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		return r, err
	}
	if err = syncDirectory(filepath.Dir(dir)); err != nil {
		return r, err
	}
	if _, err = exclusive(filepath.Join(dir, "generated.cfg"), []byte(expanded.CFG)); err != nil {
		return r, err
	}
	if _, err = exclusive(r.LogPath, nil); err != nil {
		return r, err
	}
	r.CFGCreated, err = exclusive(r.CFGPath, []byte(expanded.CFG))
	if err != nil {
		return r, errors.Join(err, s.Save())
	}
	r.Prepared = true
	return r, s.Save()
}
func exclusive(path string, b []byte) (bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return false, err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return true, err
	}
	if closeErr != nil {
		return true, closeErr
	}
	return true, syncDirectory(filepath.Dir(path))
}

// CleanupRun removes only the exact generated cfg whose content still matches.
// No recursive deletion is used. Logs and the archived snapshot are retained.
func (s *Store) CleanupRun(in *Instance, r *Run) error {
	if in.Process != "stopped" {
		return fmt.Errorf("process exit not confirmed")
	}
	if !validID(in.ID, "i") || r.Generation < 1 || s.State.Instances[in.ID] != in {
		return fmt.Errorf("invalid ownership")
	}
	owned := false
	for _, candidate := range in.Runs {
		if candidate == r {
			owned = true
		}
	}
	if !owned {
		return fmt.Errorf("unowned run")
	}
	expected := filepath.Join(in.Snapshot.CFG.Directory, fmt.Sprintf("d2core_%s_g%d.cfg", in.ID, r.Generation))
	if r.CFGPath != expected {
		return fmt.Errorf("cfg ownership path mismatch")
	}
	if r.CFGRemoved {
		return nil
	}
	st, err := os.Lstat(expected)
	if errors.Is(err, os.ErrNotExist) {
		if err = syncDirectory(filepath.Dir(expected)); err != nil {
			return err
		}
		r.CFGRemoved = true
		return s.Save()
	}
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("cfg replaced by non-regular file")
	}
	b, err := os.ReadFile(expected)
	if err != nil {
		return err
	}
	if digest(b) != r.CFGDigest {
		return fmt.Errorf("cfg changed; refusing removal")
	}
	if err = os.Remove(expected); err != nil {
		return err
	}
	if err = syncDirectory(filepath.Dir(expected)); err != nil {
		return err
	}
	r.CFGCreated = true
	r.CFGRemoved = true
	return s.Save()
}
