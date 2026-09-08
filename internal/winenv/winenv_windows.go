package winenv

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const isWindows = true

// IsAdmin сообщает, состоит ли токен процесса в группе администраторов
// с включённой привилегией (то есть процесс реально elevated).
func IsAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY, 2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0, &sid)
	if err != nil {
		return false
	}
	defer func() { _ = windows.FreeSid(sid) }()

	member, err := windows.Token(0).IsMember(sid)
	return err == nil && member
}

// Ключи, куда Evergreen-runtime прописывает свою версию: сначала
// машинная установка (WOW6432Node на x64), затем per-user.
var webView2Keys = []struct {
	root registry.Key
	path string
}{
	{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`},
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`},
	{registry.CURRENT_USER, `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`},
}

func webView2Version() string {
	for _, k := range webView2Keys {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			continue
		}
		v, _, err := key.GetStringValue("pv")
		_ = key.Close()
		// "0.0.0.0" пишется, когда runtime удалён, но ключ остался.
		if err == nil && v != "" && v != "0.0.0.0" {
			return v
		}
	}
	return ""
}
