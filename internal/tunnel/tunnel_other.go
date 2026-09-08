//go:build !windows

package tunnel

// Вне Windows туннеля нет: сборка под другие ОС нужна только для тестов.
type impl struct{}

func up(*Config, Options) (*impl, error) { return nil, ErrUnsupported }
func down(*impl)                         {}
func stats(*impl) (Stats, bool)          { return Stats{}, false }
