//go:build !windows

package singleton

// Вне Windows ограничения нет: сборка под другие ОС нужна для тестов.
type instanceImpl struct{}

func acquire(string) (*Instance, bool)    { return &Instance{}, true }
func (i *Instance) onSecondLaunch(func()) {}
func (i *Instance) release()              {}
func signal(string) error                 { return nil }
