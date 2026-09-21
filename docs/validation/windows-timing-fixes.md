# Windows timing fixes after v0.1.0

Scope: only the two failures captured in [CI health](ci-health.md). No release
tag/asset changes, real Dota tests, remote test hosts or protocol version changes.

## Error classification at process exit

The existing contract distinguishes confirmed early exit (`START_FAILED`) from
unverifiable identity (`IDENTITY_UNVERIFIED`, process unknown, allocation retained).
The single-code early-exit assertion is correct for a confirmed exit and remains unchanged. The same
observable fact must not acquire a different code merely because an image query
overlapped process exit.

Windows Open first waits on a native process handle, then reads creation time and
image identity. The child can exit between those calls. A failing image query was
unconditionally classified as ErrIdentity even when that same handle was now
signaled. Core mapped that erroneous engine result to IDENTITY_UNVERIFIED. A second, distinct case required correcting the test fixture, as described below.

The fix rechecks the **same retained handle** after an identity-read error. Only a
successful wait proving exit yields ErrGone (already mapped to START_FAILED by
core). A live process, wait failure, creation-time mismatch or executable mismatch
still fails identity verification. No PID reopening, sleep, retry, identity bypass
or new error code is introduced. Stop/recovery still require their existing exit
and ownership checks.

`TestOpenExitDuringIdentityRead` deterministically terminates and waits for the
child inside the identity-read boundary after the initial alive observation. The
pre-fix test failed with ErrIdentity; the actual native query after exit returned
"A device attached to the system is not functioning." The test also supports native
versions retaining image metadata by injecting the same query-failure condition.
`TestOpenLiveIdentityFailuresRemainUnverified` checks live access errors and both
identity mismatches, with no control handle returned and the original child alive.

## Terminal operation and worker teardown

finish persists a terminal operation while holding the manager lock. The worker's
deferred finishWorker subsequently reacquires that lock and removes the worker.
A request between those actions previously saw a terminal create but was rejected
solely because its worker still existed.

The busy guard now checks the predecessor's operation state under the same lock.
A running or missing operation remains BUSY; a terminal operation may admit a
restart only after the existing lifecycle, process, storage and persistence checks.
The predecessor worker is **not discarded**: stopOrRestart still waits on old.done
before process control or spawning. Its teardown cannot delete the newer worker.
Stop cancellation, close/wait accounting, idempotency and write-error gating remain
unchanged. This implements the existing v1 statement that status determines
completion; it does not make an active operation interruptible by restart.

`TestTerminalWorkerAllowsRestartBeforeTeardown` holds the exact terminal/teardown
window without sleeps, verifies public operation success and restart acceptance,
keeps generation 1 until teardown, then requires generation 2 ready, original
idempotency replay and successful explicit stop. It deterministically failed with
BUSY before the fix. A separate test retains the running-operation BUSY guard;
the existing low-space priority test also retains its original assertions.

## Validation

Native Windows targeted regressions and the original two failing tests passed 50
repetitions each after the initial fixes. The full suite additionally exposed a
synthetic worker with no operation in the existing priority test; the final guard
retains BUSY for that case rather than dereferencing nil or allowing a mutation.

CI checks the current event SHA on Windows/Linux with full test/vet/build/race,
plus 20 race-mode repetitions of the timing regressions. Windows uses external
link mode. These are extra stress checks, not replacements for the full suite;
all failures propagate. The frozen v0.1.0 audit stays recorded in ci-health.md and
its original runs; routine CI no longer reruns known-defective historical code.
Actual run links and outcomes are reported with the delivery commit.

Protocol v1, template schema 1 and disk format 2 are unchanged. A v0.1.1 patch
release is recommended after both platform jobs pass; this work does not publish
or overwrite a release.

## Follow-up evidence: test precondition versus product race

The first fixed run [35554990970](https://github.com/L4C99/dota2-arcade-dedicated-core/actions/runs/35554990970)
passed full Windows test/vet/build/race but failed two early-exit assertions in
its added repetition step. The diagnostic run
[35555453436](https://github.com/L4C99/dota2-arcade-dedicated-core/actions/runs/35555453436)
identified the remaining branch five times: `Access is denied.; alive=true; wait=<nil>`.
That is **not** the same fact as a signaled process handle. The original fixture
started an asynchronously exiting child and assumed observation would occur after
exit. On hosted Windows the image query can be denied before exit is observable.
Returning IDENTITY_UNVERIFIED/unknown then is correct fail-closed behavior; changing
that to START_FAILED without an exit signal would weaken the existing contract.
A local 1,000-repetition race run of the unsynchronized original fixture passed,
showing why local success alone could not settle the hosted failure.

The Windows acceptance fixture now waits once on the exact created process's
native exit event before returning from its injected spawn callback. Creation time
is checked, no process is signaled, and an unsuccessful five-second event wait is
an explicit fixture failure. There is no sleep, operation retry, second accepted
error code or timeout-based claim of exit. Linux's existing fixture is unchanged.
The original START_FAILED assertion, diagnostic retention and reservation/stop
checks are unchanged. Separate deterministic engine tests continue to require
ErrIdentity for live query denial and identity mismatches, and ErrGone for query
failure after confirmed exit. The production boundary-race fix remains necessary:
the pre-fix deterministic signaled-handle test fails independently of this fixture.

This distinction follows the native contract: [process termination signals its
process object](https://learn.microsoft.com/en-us/windows/win32/procthread/terminating-a-process).
An exit timestamp alone is not used: [GetProcessTimes documents exit-time output as
undefined before exit](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-getprocesstimes).
This resolution retains protocol v1 semantics rather than broadening error acceptance.