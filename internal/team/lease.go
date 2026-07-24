package team

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type threadLeaseRecord struct {
	PID        int    `json:"pid"`
	Token      string `json:"token"`
	AcquiredAt string `json:"acquired_at"`
}

type ThreadLease struct {
	path  string
	token string
}

func (s *Store) AcquireLease() (*ThreadLease, error) {
	if s == nil {
		return nil, fmt.Errorf("team store is required")
	}
	path := filepath.Join(s.Dir, "thread.lock")
	record := threadLeaseRecord{PID: os.Getpid(), Token: newLeaseToken(), AcquiredAt: time.Now().UTC().Format(time.RFC3339Nano)}
	for attempt := 0; attempt < 2; attempt++ {
		data, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			if _, writeErr := file.Write(append(data, '\n')); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return nil, fmt.Errorf("write team thread lease: %w", writeErr)
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(path)
				return nil, fmt.Errorf("close team thread lease: %w", closeErr)
			}
			return &ThreadLease{path: path, token: record.Token}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire team thread lease: %w", err)
		}
		var owner threadLeaseRecord
		if readErr := readJSON(path, &owner); readErr == nil && processAlive(owner.PID) {
			return nil, fmt.Errorf("team thread is locked by process %d since %s", owner.PID, owner.AcquiredAt)
		}
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, fmt.Errorf("remove stale team thread lease: %w", removeErr)
		}
	}
	return nil, fmt.Errorf("team thread lease contention")
}

func (l *ThreadLease) Release() error {
	if l == nil || strings.TrimSpace(l.path) == "" {
		return nil
	}
	var current threadLeaseRecord
	if err := readJSON(l.path, &current); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read team thread lease: %w", err)
	}
	if current.Token != l.token {
		return fmt.Errorf("team thread lease ownership changed")
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("release team thread lease: %w", err)
	}
	l.path = ""
	return nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func liveThreadLease(dir string) bool {
	var owner threadLeaseRecord
	if err := readJSON(filepath.Join(dir, "thread.lock"), &owner); err != nil {
		return false
	}
	return processAlive(owner.PID)
}

func newLeaseToken() string {
	return strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}
