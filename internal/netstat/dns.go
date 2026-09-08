package netstat

// SystemDNS возвращает DNS-серверы физических адаптеров.
//
// Нужен, чтобы ядро продолжало резолвить через сеть провайдера, когда
// туннель уже поднят: иначе система отдаёт ему DNS туннеля, а туннель без
// работающего ядра не поднимется.
func SystemDNS(excludeAdapter string) []string { return systemDNS(excludeAdapter) }
