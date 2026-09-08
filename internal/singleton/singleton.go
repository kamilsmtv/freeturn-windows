// Package singleton не даёт запустить вторую копию приложения.
//
// Две копии подрались бы за один локальный порт ядра и повесили бы в трей
// два одинаковых значка. При этом вторая копия не просто выходит: она
// будит первую, чтобы та показала окно - иначе двойной клик по ярлыку
// выглядит как «ничего не произошло».
package singleton

// Instance - признак единственного экземпляра, занятый этим процессом.
type Instance struct{ impl instanceImpl }

// Acquire занимает признак. ok=false означает, что приложение уже запущено.
func Acquire(name string) (*Instance, bool) { return acquire(name) }

// OnSecondLaunch вызывает f, когда запускают ещё одну копию приложения.
func (i *Instance) OnSecondLaunch(f func()) { i.onSecondLaunch(f) }

// Release освобождает признак при выходе.
func (i *Instance) Release() { i.release() }

// Signal будит уже запущенный экземпляр. Вызывается второй копией.
func Signal(name string) error { return signal(name) }
