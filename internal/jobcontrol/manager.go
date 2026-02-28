package jobcontrol

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"unsafe"
)

type Status int

const (
	StatusRunning Status = iota
	StatusStopped
	StatusDone
	StatusKilled
)

func (s Status) String() string {
	switch s {
	case StatusRunning:
		return "Running"
	case StatusStopped:
		return "Stopped"
	case StatusDone:
		return "Done"
	case StatusKilled:
		return "Killed"
	}
	return "Unknown"
}

type Job struct {
	ID       int
	PID      int
	PGID     int
	Cmd      string
	Status   Status
	ExitCode int
	Procs    []*os.Process
}

func (j *Job) String() string {
	return fmt.Sprintf("[%d] %d  %-10s  %s", j.ID, j.PID, j.Status, j.Cmd)
}

type Manager struct {
	mu     sync.Mutex
	jobs   map[int]*Job
	nextID int
	termFd int
}

func New() *Manager {
	return &Manager{
		jobs:   make(map[int]*Job),
		nextID: 1,
		termFd: int(os.Stdin.Fd()),
	}
}

func (m *Manager) Add(pid int, pgid int, cmd string, procs []*os.Process) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	job := &Job{ID: m.nextID, PID: pid, PGID: pgid, Cmd: cmd, Status: StatusRunning, Procs: procs}
	m.jobs[m.nextID] = job
	m.nextID++
	return job
}

func (m *Manager) Get(id int) (*Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	return j, ok
}

func (m *Manager) GetByPGID(pgid int) (*Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.PGID == pgid {
			return j, true
		}
	}
	return nil, false
}

func (m *Manager) Last() *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	var last *Job
	for _, j := range m.jobs {
		if last == nil || j.ID > last.ID {
			last = j
		}
	}
	return last
}

func (m *Manager) List() []*Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*Job
	for _, j := range m.jobs {
		if j.Status != StatusDone && j.Status != StatusKilled {
			list = append(list, j)
		}
	}
	return list
}

func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.jobs, id)
}

func (m *Manager) UpdateStatus() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, job := range m.jobs {
		if job.Status != StatusRunning {
			continue
		}
		var ws syscall.WaitStatus
		pid, err := syscall.Wait4(job.PID, &ws, syscall.WNOHANG|syscall.WUNTRACED, nil)
		if err != nil || pid == 0 {
			continue
		}
		if ws.Exited() {
			job.Status = StatusDone
			job.ExitCode = ws.ExitStatus()
		} else if ws.Stopped() {
			job.Status = StatusStopped
		} else if ws.Signaled() {
			job.Status = StatusKilled
		}
	}
}

func (m *Manager) Fg(id int) error {
	m.mu.Lock()
	job, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("job %d não encontrado", id)
	}
	if job.Status == StatusDone {
		return fmt.Errorf("job %d já terminou", id)
	}
	fmt.Printf("%s\n", job.Cmd)
	if job.PGID > 0 {
		tcsetpgrp(m.termFd, job.PGID)
	}
	if job.Status == StatusStopped {
		syscall.Kill(-job.PGID, syscall.SIGCONT)
	}
	job.Status = StatusRunning
	var ws syscall.WaitStatus
	_, err := syscall.Wait4(job.PID, &ws, syscall.WUNTRACED, nil)
	shellPGID := syscall.Getpgrp()
	tcsetpgrp(m.termFd, shellPGID)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if ws.Exited() {
		job.Status = StatusDone
		job.ExitCode = ws.ExitStatus()
	} else if ws.Stopped() {
		job.Status = StatusStopped
		fmt.Printf("\n[%d]  Stopped  %s\n", job.ID, job.Cmd)
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) Bg(id int) error {
	m.mu.Lock()
	job, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("job %d não encontrado", id)
	}
	if job.Status != StatusStopped {
		return fmt.Errorf("job %d não está parado", id)
	}
	fmt.Printf("[%d] %s &\n", job.ID, job.Cmd)
	job.Status = StatusRunning
	return syscall.Kill(-job.PGID, syscall.SIGCONT)
}

func (m *Manager) PrintCompleted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	var done []int
	for id, job := range m.jobs {
		if job.Status == StatusDone || job.Status == StatusKilled {
			fmt.Printf("[%d]  %s  %s\n", job.ID, job.Status, job.Cmd)
			done = append(done, id)
		}
	}
	for _, id := range done {
		delete(m.jobs, id)
	}
}

func SetupSignals() {
	signal.Ignore(syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU)
}

func InitShellProcessGroup() {
	pid := os.Getpid()
	pgid := syscall.Getpgrp()
	if pid != pgid {
		syscall.Setpgid(pid, pid)
	}
	termFd := int(os.Stdin.Fd())
	for i := 0; i < 10; i++ {
		tpgid, err := ioctlGetPgrp(termFd)
		if err != nil || tpgid == pid {
			break
		}
		syscall.Kill(-pid, syscall.SIGTTIN)
	}
}

func tcsetpgrp(fd, pgid int) error {
	p := pgid
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(fd),
		syscall.TIOCSPGRP,
		uintptr(unsafe.Pointer(&p)),
	)
	if errno != 0 {
		return errno
	}
	return nil
}

func ioctlGetPgrp(fd int) (int, error) {
	var pgid int
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(fd),
		syscall.TIOCGPGRP,
		uintptr(unsafe.Pointer(&pgid)),
	)
	if errno != 0 {
		return 0, errno
	}
	return pgid, nil
}
