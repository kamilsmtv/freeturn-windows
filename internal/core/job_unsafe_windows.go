package core

import "unsafe"

// Обёртки над unsafe держат небезопасные операции в одном месте:
// SetInformationJobObject принимает нетипизированный указатель и размер.
func unsafePointer[T any](v *T) unsafe.Pointer { return unsafe.Pointer(v) }

func unsafeSizeof[T any](v T) uintptr { return unsafe.Sizeof(v) }
