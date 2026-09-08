// Package updater обновляет бинарь ядра из релизов GitHub и проверяет
// наличие новой версии самого GUI.
package updater

import (
	"strconv"
	"strings"
)

// NormalizeVersion срезает префикс "v" и лишние пробелы: теги ядра выглядят
// как v1.2.3, а сам бинарь печатает 1.2.3.
func NormalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// CompareVersions сравнивает семантические версии: -1, 0 или 1.
//
// Числовые части сравниваются как числа, поэтому 1.10.0 новее 1.9.0.
// Пререлиз (1.2.0-rc1) считается старше своего релиза (1.2.0) - так же, как
// в semver. Нечисловые версии ("dev", "") считаются самыми старыми.
func CompareVersions(a, b string) int {
	an, apre := splitVersion(NormalizeVersion(a))
	bn, bpre := splitVersion(NormalizeVersion(b))

	for i := 0; i < len(an) || i < len(bn); i++ {
		x, y := part(an, i), part(bn, i)
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case apre == bpre:
		return 0
	case apre == "":
		return 1 // релиз новее своего пререлиза
	case bpre == "":
		return -1
	case apre < bpre:
		return -1
	default:
		return 1
	}
}

// IsNewer сообщает, что candidate новее current.
func IsNewer(candidate, current string) bool {
	// Неизвестная текущая версия (бинарь не опрошен) - повод предложить обновление.
	if strings.TrimSpace(current) == "" {
		return true
	}
	return CompareVersions(candidate, current) > 0
}

// splitVersion делит "1.2.3-rc1" на числовые части и пререлиз.
func splitVersion(v string) ([]int, string) {
	pre := ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		pre, v = v[i+1:], v[:i]
	}
	var nums []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			// "dev", "snapshot" и прочее: версия без номера, считаем нулём.
			nums = append(nums, 0)
			continue
		}
		nums = append(nums, n)
	}
	return nums, pre
}

func part(nums []int, i int) int {
	if i < len(nums) {
		return nums[i]
	}
	return 0
}
