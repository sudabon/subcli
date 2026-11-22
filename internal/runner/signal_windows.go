//go:build windows

package runner

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type processGroup struct {
	job windows.Handle
	pid uint32
}

func startProcessGroup(cmd *exec.Cmd) (processGroup, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return processGroup{}, err
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	if err != nil {
		windows.CloseHandle(job)
		return processGroup{}, err
	}

	// Assign the process before it can run or create children outside the job.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		windows.CloseHandle(job)
		return processGroup{}, err
	}
	pid := uint32(cmd.Process.Pid)
	var assignErr error
	err = cmd.Process.WithHandle(func(handle uintptr) {
		assignErr = windows.AssignProcessToJobObject(job, windows.Handle(handle))
	})
	err = errors.Join(err, assignErr)
	if err == nil {
		err = resumeMainThread(pid)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		windows.CloseHandle(job)
		_ = cmd.Wait()
		return processGroup{}, fmt.Errorf("starting process in Windows job: %w", err)
	}
	return processGroup{job: job, pid: pid}, nil
}

// Go closes the primary thread handle before Cmd.Start returns. A newly
// created, still-suspended process has only its primary thread.
func resumeMainThread(pid uint32) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snap)
	var entry windows.ThreadEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Thread32First(snap, &entry); err == nil; err = windows.Thread32Next(snap, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(thread)
		_, err = windows.ResumeThread(thread)
		return err
	}
	return err
}

func (g processGroup) close() { windows.CloseHandle(g.job) }

func (g processGroup) terminate(done <-chan struct{}, grace time.Duration) {
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, g.pid); err == nil {
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-done:
			return
		case <-timer.C:
		}
	}
	_ = windows.TerminateJobObject(g.job, 1)
}

// ExitCodeFromError returns a child process's exit status.
func ExitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
	}
	return 1
}
