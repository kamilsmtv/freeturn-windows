package main

import "testing"

// Служебные сообщения устройства идут сотнями на каждый подъём туннеля и
// вытесняют из журнала то, ради чего его открывают.
func TestTunnelLogFiltersNoise(t *testing.T) {
	noise := []string{
		"Routine: encryption worker 12 - started",
		"UAPI: Updating private key",
		"UDP bind has been updated",
	}
	keep := []string{
		"peer(HEsq…OWmo) - Received handshake response",
		"peer(HEsq…OWmo) - Handshake did not complete after 5 seconds, retrying (try 2)",
		"Interface state was Down, requested Up, now Up",
		"ошибка туннеля: something broke",
	}

	for _, line := range noise {
		if !isTunnelNoise(line) {
			t.Errorf("строка должна отсеиваться: %q", line)
		}
	}
	for _, line := range keep {
		if isTunnelNoise(line) {
			t.Errorf("строка должна попадать в журнал: %q", line)
		}
	}
}
