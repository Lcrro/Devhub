package process

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sync"
)

type RingLog struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func NewRingLog(max int) *RingLog { return &RingLog{max: max} }
func (r *RingLog) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.max {
		r.buf = append([]byte(nil), r.buf[len(r.buf)-r.max:]...)
	}
	return len(p), nil
}
func (r *RingLog) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(bytes.Clone(r.buf))
}

type running struct {
	cmd *exec.Cmd
	log *RingLog
}

type Manager struct {
	mu    sync.Mutex
	items map[string]*running
	logs  map[string]*RingLog
}

func New() *Manager { return &Manager{items: map[string]*running{}, logs: map[string]*RingLog{}} }

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", command)
	}
	return exec.CommandContext(ctx, "sh", "-lc", command)
}

func (m *Manager) Start(id, command, dir string) (int, error) {
	if command == "" {
		return 0, fmt.Errorf("no start command configured")
	}
	m.mu.Lock()
	if current, ok := m.items[id]; ok && current.cmd.Process != nil {
		pid := current.cmd.Process.Pid
		m.mu.Unlock()
		return pid, fmt.Errorf("project is already running under DevHub")
	}
	log := NewRingLog(256 * 1024)
	m.logs[id] = log
	m.mu.Unlock()

	cmd := shellCommand(context.Background(), command)
	if dir != "" {
		cmd.Dir = dir
	}
	configureCommand(cmd)
	cmd.Stdout = io.MultiWriter(log)
	cmd.Stderr = io.MultiWriter(log)
	if err := cmd.Start(); err != nil {
		return 0, err
	}

	m.mu.Lock()
	m.items[id] = &running{cmd: cmd, log: log}
	m.mu.Unlock()

	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if cur, ok := m.items[id]; ok && cur.cmd == cmd {
			delete(m.items, id)
		}
		m.mu.Unlock()
	}()
	return cmd.Process.Pid, nil
}

func (m *Manager) Stop(id string, pid int) error {
	m.mu.Lock()
	cur := m.items[id]
	m.mu.Unlock()
	if cur != nil && cur.cmd.Process != nil {
		return terminateOwned(cur.cmd)
	}
	if pid <= 0 {
		return fmt.Errorf("service is not running")
	}
	return terminateExternal(pid)
}

func (m *Manager) Logs(id string) string {
	m.mu.Lock()
	log := m.logs[id]
	m.mu.Unlock()
	if log == nil {
		return ""
	}
	return log.String()
}
