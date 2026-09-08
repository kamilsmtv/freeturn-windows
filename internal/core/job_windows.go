package core

import (
	"os"

	"golang.org/x/sys/windows"
)

// jobObject держит ядро привязанным к жизни GUI: при закрытии хэндла - в том
// числе когда GUI сняли taskkill'ом или он упал - Windows убивает процессы job'а.
// Это страховка на случай, если обычная остановка не отработала.
type jobObject struct{ handle windows.Handle }

func newJob() (*jobObject, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafePointer(&info)),
		uint32(unsafeSizeof(info)),
	)
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	return &jobObject{handle: h}, nil
}

// assign помещает уже запущенный процесс в job.
func (j *jobObject) assign(p *os.Process) error {
	if j == nil {
		return nil
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	return windows.AssignProcessToJobObject(j.handle, h)
}

func (j *jobObject) close() {
	if j == nil {
		return
	}
	_ = windows.CloseHandle(j.handle)
}
