//go:build !windows

package secrets

import "errors"

// Вне Windows DPAPI нет: значения остаются открытыми. Эта ветка нужна
// только для сборки тестов - боевая цель у приложения одна.
func protect([]byte) ([]byte, error) {
	return nil, errors.New("DPAPI доступен только в Windows")
}

func unprotect([]byte) ([]byte, error) {
	return nil, errors.New("DPAPI доступен только в Windows")
}
