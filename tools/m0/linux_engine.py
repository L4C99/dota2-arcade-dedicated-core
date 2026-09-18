#!/usr/bin/env python3
"""Ubuntu M0 experiment in /srv/d2core-m0 only; not the production manager.

Launch with the documented isolated systemd unit. Control uses a pidfd opened
before checking boot ID, start ticks, executable, argv and owned cgroup.
"""
import argparse
import json
import os
from pathlib import Path
import re
import select
import signal
import stat
import subprocess
import sys
import time

ROOT = Path('/srv/d2core-m0')


def save(path, value):
    # Experiments never overwrite evidence from an earlier attempt.
    with path.open('x', encoding='utf-8') as out:
        json.dump(value, out, indent=2)
        out.write('\n')
        out.flush()
        os.fsync(out.fileno())


def identity(pid):
    proc = Path('/proc') / str(pid)
    fields = (proc / 'stat').read_text().rsplit(')', 1)[1].split()
    return {
        'pid': pid,
        'bootId': Path('/proc/sys/kernel/random/boot_id').read_text().strip(),
        'startTicks': fields[19],
        'executable': os.readlink(proc / 'exe'),
        'arguments': (proc / 'cmdline').read_bytes().decode().rstrip('\0').split('\0'),
        'cgroup': (proc / 'cgroup').read_text().strip(),
    }


def validate_owned(actual, config):
    expected_cfg = config['runId'] + '.cfg'
    args = actual['arguments']
    if actual['executable'] != str(Path(config['executable']).resolve()):
        raise RuntimeError('Executable mismatch; no action taken')
    if not actual['cgroup'].endswith('/' + config['unit']):
        raise RuntimeError('Cgroup mismatch; no action taken')
    if '+exec' not in args or args[args.index('+exec') + 1] != expected_cfg:
        raise RuntimeError('Run marker mismatch; no action taken')
    if '-port' not in args or args[args.index('-port') + 1] != str(config['port']):
        raise RuntimeError('Port argument mismatch; no action taken')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['launch', 'exec', 'capture', 'observe', 'quit', 'term', 'kill'])
    parser.add_argument('run_directory', type=Path)
    parser.add_argument('--fault-after-start-before-identity', action='store_true')
    opts = parser.parse_args()
    run = opts.run_directory.resolve()
    if run.parent != ROOT / 'runs' or not re.fullmatch(r'[a-z0-9_]+', run.name):
        raise RuntimeError('Run must be a direct child of the dedicated M0 runs directory')
    config = json.loads((run / 'input.json').read_text())
    if not re.fullmatch(r'd2core_m0_[a-z0-9_]+', config['runId']):
        raise RuntimeError('Invalid run marker')
    if config['unit'] != 'd2core-m0-' + run.name + '.service':
        raise RuntimeError('Invalid owned unit name')
    if config['port'] not in (40000, 40001, 40002):
        raise RuntimeError('Outside user-reserved M0 ports')
    if not re.fullmatch(r'[a-zA-Z0-9_]+', config['map']):
        raise RuntimeError('Invalid map')
    fifo = run / 'stdin.fifo'
    log_path = Path(config.get('logPath', str(run / 'engine.log'))).resolve()
    if run not in log_path.parents:
        raise RuntimeError('Log must remain inside the owned run directory')
    if opts.action in ('launch', 'exec') and not str(log_path).isascii():
        raise RuntimeError('Engine log path must use ASCII characters; no fallback directory is selected')
    if opts.action == 'launch':
        if os.geteuid() != 0:
            raise RuntimeError('Isolated M0 unit launch requires the authorized root session')
        listeners = subprocess.check_output(['ss', '-H', '-lntu'], text=True)
        if re.search(r':' + str(config['port']) + r'\s', listeners):
            raise RuntimeError('Port already bound; no launch attempted')
        mem = Path('/proc/meminfo').read_text()
        if int(re.search(r'MemAvailable:\s+(\d+)', mem).group(1)) < 6 * 1024 * 1024:
            raise RuntimeError('Insufficient memory reserve; no launch attempted')
        properties = {
            'User': 'dota', 'Group': 'dota', 'WorkingDirectory': config['workingDirectory'],
            'ProtectSystem': 'strict', 'ReadWritePaths': str(ROOT),
            'BindReadOnlyPaths': '/srv/dota/server /srv/dota/steamcmd ' + str(ROOT / 'addons') + ':/srv/dota/server/game/dota_addons',
            'BindPaths': str(ROOT / 'engine-cfg') + ':/srv/dota/server/game/dota/cfg',
            'ProtectHome': 'yes', 'PrivateTmp': 'yes', 'NoNewPrivileges': 'yes',
            'CPUQuota': '150%', 'MemoryMax': '4G', 'MemorySwapMax': '0',
            'TasksMax': '128', 'Nice': '10', 'IOWeight': '10', 'LimitCORE': '0',
            'RuntimeMaxSec': '1800', 'TimeoutStopSec': '15', 'Restart': 'no',
            'StandardOutput': 'append:' + str(run / 'console.log'),
            'StandardError': 'append:' + str(run / 'console.log'),
        }
        command = ['systemd-run', '--unit=' + config['unit'], '--service-type=exec']
        for key, value in properties.items():
            command += ['-p', key + '=' + value]
        command += ['--setenv=HOME=' + str(ROOT / 'home'),
                    '--setenv=LD_LIBRARY_PATH=/srv/dota/steamcmd/linux64:/srv/dota/server/game/bin/linuxsteamrt64',
                    '/usr/bin/python3', str(ROOT / 'tools/linux_engine.py'), 'exec', str(run)]
        save(run / 'launch-request.json', {'at': time.time(), 'command': command})
        subprocess.run(command, check=True)
        if opts.fault_after_start_before_identity:
            os._exit(73)
        for attempt in range(100):
            pid = int(subprocess.check_output(['systemctl', 'show', config['unit'],
                                              '--property=MainPID', '--value'], text=True))
            try:
                if pid > 0 and os.readlink('/proc/' + str(pid) + '/exe') == str(Path(config['executable']).resolve()):
                    break
            except FileNotFoundError:
                pass
            time.sleep(0.1)
        opts.action = 'capture'
    if opts.action == 'exec':
        if config['addon'] not in ('3564393242', 'd2core_m0_3564393242.vpk', str(ROOT / 'assets/3564393242.vpk')):
            raise RuntimeError('Only the user-provided M0 addon is allowed')
        # This code runs as the actual engine user, not the root launcher.
        with log_path.open('x', encoding='utf-8'):
            pass
        cfg_name = config['runId'] + '.cfg'
        cfg = (f'hostname "{config["runId"]}"\n'
               f'map {config["map"]} gamemode=15 customgamemode="{config["addon"]}" nomapvalidation=1\n'
               'sv_hibernate_when_empty 0\n')
        args = [config['executable'], '-dedicated', '-allow_no_lobby_connect',
                '-ip', '0.0.0.0', '-port', str(config['port']),
                '-con_logfile', str(log_path), '+exec', cfg_name]
        save(run / 'intent.json', {'at': time.time(), 'arguments': args, 'cfg': cfg})
        for path in [run / 'startup.cfg', ROOT / 'engine-cfg' / cfg_name]:
            with path.open('x', encoding='utf-8') as out:
                out.write(cfg)
        os.mkfifo(fifo, 0o600)
        fd = os.open(fifo, os.O_RDWR | os.O_NOFOLLOW)
        os.dup2(fd, 0, inheritable=True)
        if fd != 0:
            os.close(fd)
        os.chdir(config['workingDirectory'])
        os.execv(args[0], args)
    if opts.action == 'capture':
        pid = int(subprocess.check_output(['systemctl', 'show', config['unit'],
                                          '--property=MainPID', '--value'], text=True))
        if pid <= 0:
            raise RuntimeError('No running MainPID')
        fd = os.pidfd_open(pid)
        try:
            actual = identity(pid)
            validate_owned(actual, config)
            save(run / 'process.json', actual)
            print(json.dumps(actual))
        finally:
            os.close(fd)
        return
    expected = json.loads((run / 'process.json').read_text())
    fd = os.pidfd_open(expected['pid'])
    try:
        actual = identity(expected['pid'])
        if actual != expected:
            raise RuntimeError('Persisted identity mismatch; no action taken')
        validate_owned(actual, config)
        if opts.action == 'observe':
            print(json.dumps({'at': time.time(), 'identity': actual,
                              'logBytes': log_path.stat().st_size
                              if log_path.exists() else 0}))
            return
        started = time.monotonic()
        if opts.action == 'quit':
            if not stat.S_ISFIFO(fifo.lstat().st_mode):
                raise RuntimeError('Not an owned FIFO')
            writer = os.open(fifo, os.O_WRONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
            try:
                if os.write(writer, b'quit\n') != 5:
                    raise RuntimeError('Incomplete quit write')
            finally:
                os.close(writer)
        else:
            signal.pidfd_send_signal(fd, signal.SIGTERM if opts.action == 'term' else signal.SIGKILL)
        poller = select.poll()
        poller.register(fd, select.POLLIN)
        exited = bool(poller.poll(10000))
        result = {'at': time.time(), 'method': opts.action, 'exited': exited,
                  'elapsedSeconds': time.monotonic() - started, 'identity': expected}
        save(run / (opts.action + '-result.json'), result)
        print(json.dumps(result))
        if not exited:
            raise RuntimeError('Exit unconfirmed; evidence retained, no automatic force fallback')
    finally:
        os.close(fd)


if __name__ == '__main__':
    try:
        main()
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        sys.exit(1)
