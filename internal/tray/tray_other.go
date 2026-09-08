//go:build !windows

package tray

// Вне Windows значка нет: сборка под другие ОС нужна только для тестов.
type impl struct{}

func (t *Tray) start()         {}
func (t *Tray) setState(State) {}
func (t *Tray) stop()          {}
