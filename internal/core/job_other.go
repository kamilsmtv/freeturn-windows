//go:build !windows

package core

import "os"

// Вне Windows job-объектов нет: заглушки нужны только для сборки тестов.
type jobObject struct{}

func newJob() (*jobObject, error)           { return &jobObject{}, nil }
func (*jobObject) assign(*os.Process) error { return nil }
func (*jobObject) close()                   {}
