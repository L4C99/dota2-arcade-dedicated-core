// Package core implements the single-writer local instance lifecycle. Transport
// adapters call Respond; they do not own server processes or operation lifetime.
package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
)

type worker struct {
	kind        string
	operationID string
	cancel      chan struct{}
	done        chan struct{}
}
type Manager struct {
	mu         sync.Mutex
	wg         sync.WaitGroup
	store      *records.Store
	workers    map[string]*worker
	closed     chan struct{}
	isClosed   bool
	writeError error
	persist    func() error
	ports      records.PortRange
	options    Options
	freeBytes  func(string) (uint64, error)
	storage    storageStatus
	spawn      func(engine.Spec) (engine.Identity, error)
	clock      func() time.Time
	bindings   func(*engine.Handle) ([]engine.Binding, error)
}
type request struct {
	ProtocolVersion int             `json:"protocolVersion"`
	Method          string          `json:"method"`
	Params          json.RawMessage `json:"params"`
}
type response struct {
	ProtocolVersion int              `json:"protocolVersion"`
	OK              bool             `json:"ok"`
	Result          any              `json:"result,omitempty"`
	Error           *records.Failure `json:"error,omitempty"`
}

// Open requires an already-held localipc data-directory lock. Existing runs
// are identity-checked before observation; ambiguous intent is never respawned.
func Open(dir string) (*Manager, error) {
	return OpenWithPorts(dir, records.DefaultPortRange())
}

func OpenWithPorts(dir string, ports records.PortRange) (*Manager, error) {
	o := DefaultOptions()
	o.Ports = ports
	return OpenWithOptions(dir, o)
}

func OpenWithOptions(dir string, options Options) (*Manager, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	s, err := records.Open(dir)
	if err != nil {
		return nil, err
	}
	m := &Manager{store: s, workers: map[string]*worker{}, closed: make(chan struct{}), persist: s.Save, ports: options.Ports, options: options, freeBytes: records.FreeBytes}
	m.spawn = engine.Start
	if err = m.recover(); err != nil {
		return nil, err
	}
	m.wg.Add(1)
	go m.watch()
	return m, nil
}
func (m *Manager) Close() {
	m.mu.Lock()
	if !m.isClosed {
		m.isClosed = true
		close(m.closed)
	}
	// No engine signaling here. Workers preserve durable in-flight operations.
	m.mu.Unlock()
	m.wg.Wait() // Retain the external data-dir lock until all writes have ceased.
}
func (m *Manager) save() error {
	if m.writeError != nil {
		return m.writeError
	}
	if err := m.persist(); err != nil {
		m.writeError = err
		return err
	}
	return nil
}
func (m *Manager) failedSave(in *records.Instance, w *worker, err error) {
	f := fail("IO_ERROR", "persist", err.Error())
	f.InstanceID = in.ID
	f.OperationID = w.operationID
	in.Error = f
	in.Lifecycle = "failed"
	op := m.store.State.Operations[w.operationID]
	in.UpdatedAt = m.recordTime(in, op)
	op.Status = "failed"
	op.Phase = "done"
	op.Error = f
	now := m.recordTime(in, op)
	op.FinishedAt = &now
	// Memory reports the storage failure honestly. Do not attempt further writes
	// or new operations until the operator fixes storage and restarts the manager.
}
func fail(code, stage, message string) *records.Failure {
	return &records.Failure{Code: code, Stage: stage, Message: message}
}
func classify(err error, stage string) *records.Failure {
	var f *records.Failure
	if errors.As(err, &f) {
		copy := *f
		return &copy
	}
	var c *config.Error
	if errors.As(err, &c) {
		return fail(c.Code, stage, c.Error())
	}
	return fail("IO_ERROR", stage, err.Error())
}
func parse(raw json.RawMessage, out any) *records.Failure {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if err := config.DecodeStrict(raw, out); err != nil {
		return fail("INVALID_REQUEST", "protocol", err.Error())
	}
	return nil
}

func (m *Manager) Respond(data []byte) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	var req request
	var result any
	var failure *records.Failure
	if len(data) > 1<<20 {
		failure = fail("INVALID_REQUEST", "protocol", "request exceeds 1 MiB")
	} else if err := config.DecodeStrict(data, &req); err != nil {
		failure = fail("INVALID_REQUEST", "protocol", err.Error())
	} else if req.ProtocolVersion != 1 {
		failure = fail("UNSUPPORTED_VERSION", "protocol", "expected protocolVersion 1")
	} else {
		result, failure = m.dispatch(req)
	}
	r := response{ProtocolVersion: 1, OK: failure == nil, Result: result, Error: failure}
	b, err := json.Marshal(r)
	if err != nil {
		b = []byte(`{"protocolVersion":1,"ok":false,"error":{"code":"INTERNAL_ERROR","stage":"protocol","message":"response encoding failed"}}`)
	}
	if len(b) > 2<<20 {
		b = []byte(`{"protocolVersion":1,"ok":false,"error":{"code":"INTERNAL_ERROR","stage":"protocol","message":"response exceeds 2 MiB"}}`)
	}
	return b
}
func (m *Manager) dispatch(req request) (any, *records.Failure) {
	if m.isClosed {
		return nil, fail("MANAGER_UNAVAILABLE", "transport", "manager is stopping")
	}
	switch req.Method {
	case "create":
		var p struct {
			Template string `json:"template"`
			Port     int    `json:"port"`
			Key      string `json:"idempotencyKey"`
		}
		if e := parse(req.Params, &p); e != nil {
			return nil, e
		}
		if m.writeError != nil {
			return nil, classify(m.writeError, "persist")
		}
		in, op, err := m.store.CreateChecked(p.Template, p.Port, p.Key, m.ports, func(snapshot config.Template) error {
			for _, path := range []string{m.store.Dir, snapshot.CFG.Directory} {
				if f := m.checkSpace(path); f != nil {
					return f
				}
			}
			return nil
		})
		if err != nil {
			var uncertain *records.DurabilityError
			if errors.As(err, &uncertain) {
				// Publication may already be durable. Preserve its IDs and stop
				// accepting mutations until disk state is reloaded explicitly.
				m.writeError = err
			}
			return nil, classify(err, "validate")
		}
		// A retry returns exactly the original IDs, including historical results.
		if in.Generation == 0 && op.Status == "running" && m.workers[in.ID] == nil && in.Lifecycle == "active" {
			w := m.newWorker(in, "create", op.ID)
			go m.start(in.ID, w)
		}
		return m.accepted(in, op), nil
	case "list":
		if e := parse(req.Params, &struct{}{}); e != nil {
			return nil, e
		}
		ids := []string{}
		for id, in := range m.store.State.Instances {
			if in.Lifecycle != "reclaimed" {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		list := []any{}
		for _, id := range ids {
			list = append(list, m.snapshot(m.store.State.Instances[id]))
		}
		return map[string]any{"instances": list, "storage": m.storage}, nil
	case "status", "stop", "restart":
		var p struct {
			ID string `json:"instanceId"`
		}
		if e := parse(req.Params, &p); e != nil {
			return nil, e
		}
		in := m.store.State.Instances[p.ID]
		if in == nil {
			return nil, fail("NOT_FOUND", "validate", "unknown instance")
		}
		if req.Method == "status" {
			return m.snapshot(in), nil
		}
		if m.writeError != nil {
			return nil, classify(m.writeError, "persist")
		}
		return m.change(in, req.Method)
	case "operation":
		var p struct {
			ID string `json:"operationId"`
		}
		if e := parse(req.Params, &p); e != nil {
			return nil, e
		}
		op := m.store.State.Operations[p.ID]
		if op == nil {
			return nil, fail("NOT_FOUND", "validate", "unknown operation")
		}
		return op, nil
	case "logs":
		var p struct {
			ID         string `json:"instanceId"`
			Generation int    `json:"generation"`
			Tail       int    `json:"tail"`
		}
		if e := parse(req.Params, &p); e != nil {
			return nil, e
		}
		if p.Tail < 0 || p.Tail > 1000 {
			return nil, fail("INVALID_REQUEST", "validate", "tail must be 1..1000, or 0 for default")
		}
		in := m.store.State.Instances[p.ID]
		if in == nil {
			return nil, fail("NOT_FOUND", "validate", "unknown instance")
		}
		if p.Generation == 0 {
			p.Generation = in.Generation
		}
		if p.Tail == 0 {
			p.Tail = 100
		}
		if p.Generation < 1 || p.Generation > len(in.Runs) {
			return nil, fail("NOT_FOUND", "validate", "unknown generation")
		}
		text, truncated, err := engine.ReadTail(in.Runs[p.Generation-1].LogPath, p.Tail)
		if err != nil {
			return nil, classify(err, "observe")
		}
		return map[string]any{"generation": p.Generation, "text": text, "truncated": truncated}, nil
	default:
		return nil, fail("INVALID_REQUEST", "protocol", "unknown method")
	}
}
func (m *Manager) snapshot(in *records.Instance) map[string]any {
	var evidence *engine.Evidence
	var portCheck *records.PortCheck
	bindings := []engine.Binding{}
	if len(in.Runs) > 0 {
		r := in.Runs[len(in.Runs)-1]
		evidence = r.Evidence
		portCheck = r.PortCheck
		if r.Bindings != nil {
			bindings = r.Bindings
		}
	}
	return map[string]any{"instanceId": in.ID, "templateName": in.Snapshot.Name, "port": in.Port, "generation": in.Generation, "lifecycle": in.Lifecycle, "process": in.Process, "room": in.Room, "cleanup": in.Cleanup, "currentOperationId": in.CurrentOperationID, "createdAt": in.CreatedAt, "updatedAt": in.UpdatedAt, "evidence": evidence, "bindings": bindings, "portCheck": portCheck, "error": in.Error}
}
func (m *Manager) accepted(in *records.Instance, op *records.Operation) any {
	return map[string]any{"accepted": true, "instanceId": in.ID, "operationId": op.ID, "state": map[string]any{"lifecycle": in.Lifecycle, "process": in.Process, "room": in.Room, "cleanup": in.Cleanup, "generation": in.Generation, "operationStatus": op.Status}}
}
func (m *Manager) newWorker(in *records.Instance, kind, id string) *worker {
	w := &worker{kind: kind, operationID: id, cancel: make(chan struct{}), done: make(chan struct{})}
	m.workers[in.ID] = w
	m.wg.Add(1)
	return w
}
func cancelled(w *worker) bool {
	select {
	case <-w.cancel:
		return true
	default:
		return false
	}
}
func (m *Manager) finishWorker(id string, w *worker) {
	defer m.wg.Done()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.workers[id] == w {
		delete(m.workers, id)
	}
	close(w.done)
}
func (m *Manager) finish(in *records.Instance, w *worker, status string, e *records.Failure) {
	op := m.store.State.Operations[w.operationID]
	if op.Status != "running" {
		return
	}
	now := m.recordTime(in, op)
	op.Status = status
	op.Phase = "done"
	op.FinishedAt = &now
	op.Error = e
	in.UpdatedAt = now
	if e != nil {
		e.InstanceID = in.ID
		e.OperationID = op.ID
		in.Error = e
		in.Lifecycle = "failed"
		if in.Process == "unknown" {
			in.Room = "unknown"
		} else {
			in.Room = "failed"
		}
	}
	if err := m.save(); err != nil {
		m.failedSave(in, w, err)
	}
}
func (m *Manager) start(id string, w *worker) {
	defer m.finishWorker(id, w)
	m.launch(id, w)
}
func (m *Manager) launch(id string, w *worker) {
	m.mu.Lock()
	in := m.store.State.Instances[id]
	if m.isClosed || cancelled(w) {
		m.mu.Unlock()
		return
	}
	op := m.store.State.Operations[w.operationID]
	for _, path := range []string{m.store.Dir, in.Snapshot.CFG.Directory} {
		if f := m.checkSpace(path); f != nil {
			m.finish(in, w, "failed", f)
			m.mu.Unlock()
			return
		}
	}
	op.Phase = "preparing"
	if err := m.save(); err != nil {
		m.failedSave(in, w, err)
		m.mu.Unlock()
		return
	}
	r, err := m.store.PrepareRun(in)
	if err != nil {
		m.finish(in, w, "failed", classify(err, "persist"))
		m.mu.Unlock()
		return
	}
	m.store.State.Operations[w.operationID].Generation = r.Generation
	if err = records.CheckPort(in.Port); err != nil {
		m.finish(in, w, "failed", classify(err, "validate"))
		m.mu.Unlock()
		return
	}
	// Holding the mutex through Start makes stop's cancellation and spawning a
	// single ordering boundary. There is no launch after stop acknowledges.
	r.SpawnAttempted = true
	r.StartupDeadline = time.Now().UTC().Add(time.Duration(in.Snapshot.Timeouts.StartupSeconds) * time.Second)
	op.Phase = "spawning"
	if err = m.save(); err != nil {
		m.failedSave(in, w, err)
		m.mu.Unlock()
		return
	}
	identity, err := m.spawn(engine.Spec{Executable: in.Snapshot.Executable, WorkingDirectory: in.Snapshot.WorkingDirectory, Arguments: r.Arguments, RunDirectory: r.Directory, InputPrepared: func(input engine.Identity) error {
		r.InputOwnership = &input
		return m.save() // Must be durable before the child can inherit the FIFO.
	}})
	if identity.PID > 0 {
		r.Identity = &identity
		in.Process = "unknown"
	}
	if err != nil {
		var rollback *engine.StartError
		if identity.PID == 0 || (errors.As(err, &rollback) && rollback.Exited) {
			in.Process = "stopped"
			in.Room = "unknown"
			r.StopResult = &engine.StopResult{Confirmed: true}
		}
		m.finish(in, w, "failed", fail("START_FAILED", "spawn", err.Error()))
		m.mu.Unlock()
		return
	}
	in.Process = "running"
	in.Room = "loading"
	in.Error = nil
	op.Phase = "observing"
	if err = m.save(); err != nil {
		m.failedSave(in, w, err)
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	m.awaitReadiness(in, r, w)
}

func (m *Manager) awaitReadiness(in *records.Instance, r *records.Run, w *worker) {
	observer := engine.NewObserver(r.Generation, r.LogPath, in.Snapshot.Readiness)
	identity := *r.Identity
	deadline := r.StartupDeadline
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		m.mu.Lock()
		if m.isClosed || cancelled(w) {
			m.mu.Unlock()
			return
		}
		ready, failure := m.observe(in, r, observer)
		if failure != nil {
			m.finish(in, w, "failed", failure)
			m.mu.Unlock()
			return
		}
		if ready && !time.Now().After(deadline) {
			m.finish(in, w, "succeeded", nil)
			m.mu.Unlock()
			return
		}
		if time.Now().After(deadline) {
			m.mu.Unlock()
			result, stopErr := engine.Stop(identity, time.Duration(in.Snapshot.Timeouts.StopSeconds)*time.Second, time.Duration(in.Snapshot.Timeouts.ForceSeconds)*time.Second)
			m.mu.Lock()
			r.StopResult = &result
			if result.Confirmed {
				in.Process = "stopped"
			} else {
				in.Process = "unknown"
			}
			if r.Evidence != nil {
				r.Evidence.Valid = false
			}
			message := "startup readiness deadline exceeded"
			if stopErr != nil {
				message += "; stop: " + stopErr.Error()
			}
			m.finish(in, w, "failed", fail("START_TIMEOUT", "observe", message))
			m.mu.Unlock()
			return
		}
		m.mu.Unlock()
		select {
		case <-m.closed:
			return
		case <-w.cancel:
			return
		case <-tick.C:
		}
	}
}
func (m *Manager) observe(in *records.Instance, r *records.Run, o *engine.Observer) (ready bool, failure *records.Failure) {
	defer func() {
		if failure != nil {
			in.Room = "failed"
			if in.Process == "unknown" {
				in.Room = "unknown"
			}
			if r.Evidence != nil {
				r.Evidence.Valid = false
			}
		}
	}()
	h, err := engine.Open(*r.Identity)
	if errors.Is(err, engine.ErrGone) {
		in.Process = "stopped"
		in.Room = "failed"
		if r.Evidence != nil {
			r.Evidence.Valid = false
		}
		return false, fail("START_FAILED", "observe", "process exited")
	}
	if err != nil {
		in.Process = "unknown"
		in.Room = "unknown"
		if r.Evidence != nil {
			r.Evidence.Valid = false
		}
		return false, fail("IDENTITY_UNVERIFIED", "observe", err.Error())
	}
	defer h.Close()
	in.Process = "running"
	obs, err := o.Scan(true)
	if err != nil {
		return false, classify(err, "observe")
	}
	r.Evidence = obs.Evidence
	readBindings := m.bindings
	if readBindings == nil {
		readBindings = (*engine.Handle).Bindings
	}
	bindings, err := readBindings(h)
	// The process can exit while its log or native socket tables are read.
	// Recheck the same verified handle before classifying a /proc error or
	// promoting readiness; a successful earlier Open is not a liveness lease.
	alive, identityErr := h.Alive()
	if errors.Is(err, engine.ErrGone) || (identityErr == nil && !alive) {
		in.Process = "stopped"
		in.Room = "failed"
		if r.Evidence != nil {
			r.Evidence.Valid = false
		}
		return false, fail("START_FAILED", "observe", "process exited")
	}
	if identityErr != nil {
		in.Process = "unknown"
		in.Room = "unknown"
		if r.Evidence != nil {
			r.Evidence.Valid = false
		}
		return false, fail("IDENTITY_UNVERIFIED", "observe", identityErr.Error())
	}
	if err != nil {
		if r.Evidence != nil {
			r.Evidence.Valid = false
		}
		return false, classify(err, "observe")
	}
	r.Bindings = bindings
	r.PortCheck = m.checkBindingConflicts(in, bindings)
	if len(r.PortCheck.Conflicts) > 0 {
		return false, fail("PORT_IN_USE", "observe", r.PortCheck.Conflicts[0])
	}
	if obs.Failure != "" {
		return false, fail("START_FAILED", "observe", "matched failure signal: "+obs.Failure)
	}
	// A later clean scan does not undo a failed lifecycle or its operation.
	// Explicit stop/reclaim is still required before a new creation intention.
	if in.Lifecycle == "failed" && in.Error != nil {
		return false, in.Error
	}
	tcp, udp := false, false
	for _, b := range bindings {
		if b.Port == in.Port {
			if b.Protocol == "tcp4" || b.Protocol == "tcp6" {
				tcp = true
			}
			if b.Protocol == "udp4" || b.Protocol == "udp6" {
				udp = true
			}
		}
	}
	if obs.Ready && tcp && udp {
		in.Room = "ready"
		return true, nil
	}
	in.Room = "loading"
	return false, nil
}

func (m *Manager) change(in *records.Instance, kind string) (any, *records.Failure) {
	old := m.workers[in.ID]
	if kind == "restart" {
		if in.Lifecycle == "reclaimed" {
			return nil, fail("RECLAIMED", "validate", "historical instance cannot restart")
		}
		if old != nil {
			operation := m.store.State.Operations[old.operationID]
			if operation == nil || operation.Status == "running" {
				return nil, fail("BUSY", "validate", "instance operation is running")
			}
		}
		// A terminal operation is externally complete even if its deferred
		// worker teardown has not run. Keep old as the predecessor below:
		// stopOrRestart waits for old.done before any process control/spawn.
		if in.Lifecycle != "active" || in.Process != "running" {
			return nil, fail("INVALID_STATE", "validate", "restart requires a running active instance")
		}
		for _, path := range []string{m.store.Dir, in.Snapshot.CFG.Directory} {
			if f := m.checkSpace(path); f != nil {
				return nil, f
			}
		}
	}
	if kind == "stop" {
		if in.Lifecycle == "reclaimed" {
			op := m.store.State.Operations[in.CurrentOperationID]
			if op == nil {
				return nil, fail("IO_ERROR", "persist", "missing stop history")
			}
			return m.accepted(in, op), nil
		}
		if old != nil && old.kind == "stop" {
			return m.accepted(in, m.store.State.Operations[old.operationID]), nil
		}
	}
	id, err := records.ID("o")
	if err != nil {
		return nil, classify(err, "persist")
	}
	op := &records.Operation{ID: id, Kind: kind, InstanceID: in.ID, Generation: in.Generation, Status: "running", Phase: "accepted", CreatedAt: m.recordTime(in, nil)}
	m.store.State.Operations[id] = op
	in.CurrentOperationID = id
	if old != nil && !cancelled(old) {
		close(old.cancel)
		m.finish(in, old, "cancelled", nil)
	}
	if err = m.save(); err != nil {
		provisional := &worker{operationID: id}
		m.failedSave(in, provisional, err)
		failure := classify(err, "persist")
		failure.InstanceID = in.ID
		failure.OperationID = id
		return nil, failure
	}
	w := m.newWorker(in, kind, id)
	go m.stopOrRestart(in.ID, w, old)
	return m.accepted(in, op), nil
}
func (m *Manager) stopOrRestart(id string, w, old *worker) {
	defer m.finishWorker(id, w)
	if old != nil {
		select {
		case <-old.done:
		case <-m.closed:
			return
		}
	}
	m.mu.Lock()
	in := m.store.State.Instances[id]
	if m.isClosed || cancelled(w) {
		m.mu.Unlock()
		return
	}
	var r *records.Run
	if len(in.Runs) > 0 {
		r = in.Runs[len(in.Runs)-1]
	}
	if r != nil && r.Identity != nil && in.Process != "stopped" {
		m.store.State.Operations[w.operationID].Phase = "stopping"
		if err := m.save(); err != nil {
			m.failedSave(in, w, err)
			m.mu.Unlock()
			return
		}
		identity := *r.Identity
		grace := in.Snapshot.Timeouts.StopSeconds
		force := in.Snapshot.Timeouts.ForceSeconds
		m.mu.Unlock()
		result, err := engine.Stop(identity, time.Duration(grace)*time.Second, time.Duration(force)*time.Second)
		m.mu.Lock()
		r.StopResult = &result
		if r.Evidence != nil {
			r.Evidence.Valid = false
		}
		if !result.Confirmed {
			in.Process = "unknown"
			message := "exit not confirmed"
			if err != nil {
				message = err.Error()
			}
			m.finish(in, w, "failed", fail("STOP_FAILED", "stop", message))
			m.mu.Unlock()
			return
		}
		in.Process = "stopped"
		in.Room = "unknown"
		if err = m.save(); err != nil {
			m.failedSave(in, w, err)
			m.mu.Unlock()
			return
		}
	}
	if in.Process != "stopped" {
		m.finish(in, w, "failed", fail("IDENTITY_UNVERIFIED", "stop", "cannot confirm ownership/exit"))
		m.mu.Unlock()
		return
	}
	if m.isClosed || cancelled(w) {
		m.mu.Unlock()
		return
	}
	m.store.State.Operations[w.operationID].Phase = "cleanup"
	if err := m.save(); err != nil {
		m.failedSave(in, w, err)
		m.mu.Unlock()
		return
	}
	for _, run := range in.Runs {
		input := run.InputOwnership
		if input == nil {
			input = run.Identity
		} // Earlier format-2 complete identities.
		if input != nil && !run.InputRemoved {
			cleanup := engine.CleanupInput
			if run.Identity != nil {
				input = run.Identity
			}
			if run.StopResult != nil && run.StopResult.Confirmed {
				cleanup = engine.CleanupConfirmedInput
				if run.InputOwnership != nil {
					input = run.InputOwnership
				}
			}
			if err := cleanup(*input); err != nil {
				in.Cleanup = "failed"
				m.finish(in, w, "failed", fail("CLEANUP_FAILED", "cleanup", err.Error()))
				m.mu.Unlock()
				return
			}
			run.InputRemoved = true
			if err := m.save(); err != nil {
				m.failedSave(in, w, err)
				m.mu.Unlock()
				return
			}
		}
		if err := m.store.CleanupRun(in, run); err != nil {
			in.Cleanup = "failed"
			m.finish(in, w, "failed", fail("CLEANUP_FAILED", "cleanup", err.Error()))
			m.mu.Unlock()
			return
		}
	}
	if m.isClosed || cancelled(w) {
		m.mu.Unlock()
		return
	}
	if w.kind == "stop" {
		in.Lifecycle = "reclaimed"
		in.Cleanup = "complete"
		in.Room = "unknown"
		in.Error = nil
		m.finish(in, w, "succeeded", nil)
		m.mu.Unlock()
		return
	}
	in.Lifecycle = "active"
	in.Cleanup = "pending"
	in.Error = nil
	// start owns completion of the same worker; avoid double-closing done.
	m.mu.Unlock()
	m.launch(id, w)
}

func (m *Manager) watch() {
	defer m.wg.Done()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	observers := map[string]*engine.Observer{}
	for {
		select {
		case <-m.closed:
			return
		case <-tick.C:
		}
		m.mu.Lock()
		if m.isClosed {
			m.mu.Unlock()
			return
		}
		if time.Since(m.storage.CheckedAt) >= time.Minute {
			m.maintainStorage(time.Now().UTC())
		}
		currentObservers := map[string]bool{}
		for id, in := range m.store.State.Instances {
			if m.workers[id] != nil || in.Lifecycle == "reclaimed" || in.Process != "running" || len(in.Runs) == 0 {
				continue
			}
			r := in.Runs[len(in.Runs)-1]
			if r.Identity == nil {
				continue
			}
			key := fmt.Sprintf("%s/%d", id, r.Generation)
			currentObservers[key] = true
			o := observers[key]
			if o == nil {
				o = engine.NewObserver(r.Generation, r.LogPath, in.Snapshot.Readiness)
				observers[key] = o
			}
			oldRoom := in.Room
			oldProcess, oldLifecycle, oldError := in.Process, in.Lifecycle, in.Error
			_, failure := m.observe(in, r, o)
			if failure != nil {
				failure.InstanceID = in.ID
				failure.OperationID = in.CurrentOperationID
				in.Lifecycle = "failed"
				if m.writeError == nil {
					in.Error = failure
				}
				if oldRoom != in.Room || oldProcess != in.Process || oldLifecycle != in.Lifecycle || !sameFailure(oldError, in.Error) {
					in.UpdatedAt = m.recordTime(in, nil)
					if err := m.save(); err != nil {
						in.Error = fail("IO_ERROR", "persist", err.Error())
						in.Error.InstanceID = in.ID
						in.Error.OperationID = in.CurrentOperationID
					}
				}
			} else if oldRoom != in.Room {
				in.UpdatedAt = m.recordTime(in, nil)
				if err := m.save(); err != nil {
					in.Error = fail("IO_ERROR", "persist", err.Error())
					in.Error.InstanceID = in.ID
					in.Error.OperationID = in.CurrentOperationID
				}
			}
		}
		for key := range observers {
			if !currentObservers[key] {
				delete(observers, key)
			}
		}
		m.mu.Unlock()
	}
}
