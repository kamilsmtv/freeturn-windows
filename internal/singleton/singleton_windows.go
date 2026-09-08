package singleton

import (
	"errors"

	"golang.org/x/sys/windows"
)

type instanceImpl struct {
	mutex windows.Handle
	event windows.Handle
	name  string
}

// Имена объектов ядра: Local\ - пространство текущего сеанса входа.
func mutexName(name string) string { return `Local\` + name + `.mutex` }
func eventName(name string) string { return `Local\` + name + `.show` }

func acquire(name string) (*Instance, bool) {
	ptr, err := windows.UTF16PtrFromString(mutexName(name))
	if err != nil {
		// Без имени проверять нечего - пусть приложение просто запустится.
		return &Instance{}, true
	}

	handle, err := windows.CreateMutex(nil, false, ptr)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return &Instance{}, false
	}
	if err != nil && handle == 0 {
		// Непонятная ошибка не повод не запускаться.
		return &Instance{}, true
	}

	inst := &Instance{impl: instanceImpl{mutex: handle, name: name}}

	// Событие со сбросом вручную не годится: сигнал нужно ловить каждый раз.
	if ep, err := windows.UTF16PtrFromString(eventName(name)); err == nil {
		if ev, err := windows.CreateEvent(nil, 0, 0, ep); err == nil {
			inst.impl.event = ev
		}
	}
	return inst, true
}

func (i *Instance) onSecondLaunch(f func()) {
	if i.impl.event == 0 || f == nil {
		return
	}
	go func() {
		for {
			s, err := windows.WaitForSingleObject(i.impl.event, windows.INFINITE)
			if err != nil || s != windows.WAIT_OBJECT_0 {
				return
			}
			f()
		}
	}()
}

func (i *Instance) release() {
	if i.impl.event != 0 {
		_ = windows.CloseHandle(i.impl.event)
	}
	if i.impl.mutex != 0 {
		_ = windows.CloseHandle(i.impl.mutex)
	}
}

func signal(name string) error {
	ptr, err := windows.UTF16PtrFromString(eventName(name))
	if err != nil {
		return err
	}
	const eventModifyState = 0x0002
	handle, err := windows.OpenEvent(eventModifyState, false, ptr)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	return windows.SetEvent(handle)
}
