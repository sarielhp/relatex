//go:build windows

package command

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Group is a started program with every program it starts: on Windows a job object that holds
// them all (decision 129). Terminating the job reaches every descendant, and the job kills what
// is left when its last handle closes (KILL_ON_JOB_CLOSE), so a relatex that dies takes its
// programs with it, as Linux's parent-death signal does for the browser.
type Group struct {
	job windows.Handle
}

// StartGroup starts cmd suspended in a new process group, puts it in a new job object, and only
// then lets it run, so nothing it starts can escape the job.
func StartGroup(cmd *exec.Cmd) (*Group, error) {
	job, err := killOnCloseJob()
	if err != nil {
		return nil, err
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP
	if err := cmd.Start(); err != nil {
		_ = windows.CloseHandle(job) // nothing was started in it
		return nil, err
	}
	group := &Group{job: job}
	if err := group.admit(uint32(cmd.Process.Pid)); err != nil {
		_ = cmd.Process.Kill() // still suspended: it never ran
		group.Close()
		return nil, err
	}
	return group, nil
}

// killOnCloseJob is a new job object that kills its processes when its last handle closes. It lets
// a program that asks for it start its child outside the job (BREAKAWAY_OK): a scoop shim (mutool,
// qpdf and poppler installed by scoop) starts its program that way, in a kill-on-close job of its
// own, so the program still ends with the shim; without it the shim cannot start the program.
func killOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE |
			windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK},
	}
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		_ = windows.CloseHandle(job) // the job is unusable
		return 0, fmt.Errorf("set job object limits: %w", err)
	}
	return job, nil
}

// admit puts the suspended process in the job and resumes its threads.
func (g *Group) admit(pid uint32) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(g.job, process); err != nil {
		return fmt.Errorf("assign process %d to its job: %w", pid, err)
	}
	return resumeThreads(pid)
}

// resumeThreads resumes every thread of a process started with CREATE_SUSPENDED (its main thread).
func resumeThreads(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("list threads: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := 0
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		if err := resumeThread(entry.ThreadID); err != nil {
			return err
		}
		resumed++
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("list threads: %w", err)
	}
	if resumed == 0 {
		return fmt.Errorf("process %d has no thread to resume", pid)
	}
	return nil
}

func resumeThread(id uint32) error {
	thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, id)
	if err != nil {
		return fmt.Errorf("open thread %d: %w", id, err)
	}
	defer windows.CloseHandle(thread)
	if _, err := windows.ResumeThread(thread); err != nil {
		return fmt.Errorf("resume thread %d: %w", id, err)
	}
	return nil
}

// Kill terminates every process in the job.
func (g *Group) Kill() {
	_ = windows.TerminateJobObject(g.job, 1) // a job already empty has nothing left to kill
}

// Close closes the job's handle, which kills whatever still runs in it.
func (g *Group) Close() {
	if g.job != 0 {
		_ = windows.CloseHandle(g.job) // closing a valid handle once does not fail
		g.job = 0
	}
}

// DetachedAttributes start a program that outlives relatex's own console signals: a process group
// of its own (an editor relatex sync opens).
func DetachedAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// descendants is empty on Windows: a job holds every descendant, so killing the jobs reaches them.
func descendants(int) map[int]string { return nil }

// killDescendants has nothing to do on Windows (see descendants).
func killDescendants(map[int]string) {}
