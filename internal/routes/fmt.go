package routes

import "fmt"

// sprintf вынесен отдельно, чтобы не тянуть fmt в каждый файл пакета.
func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }
