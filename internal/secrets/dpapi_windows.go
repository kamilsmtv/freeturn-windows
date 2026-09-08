package secrets

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// entropy привязывает блоб к нашему приложению: чужой процесс под тем же
// пользователем не расшифрует значение, не зная этой соли.
var entropy = []byte("FreeTurn/1")

func protect(data []byte) ([]byte, error) {
	in := blobOf(data)
	ent := blobOf(entropy)
	var out windows.DataBlob

	err := windows.CryptProtectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	defer freeBlob(&out)
	return copyBlob(&out), nil
}

func unprotect(blob []byte) ([]byte, error) {
	in := blobOf(blob)
	ent := blobOf(entropy)
	var out windows.DataBlob

	err := windows.CryptUnprotectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	defer freeBlob(&out)
	return copyBlob(&out), nil
}

func blobOf(b []byte) windows.DataBlob {
	if len(b) == 0 {
		return windows.DataBlob{}
	}
	return windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// copyBlob переносит данные из памяти, выделенной LocalAlloc, в срез Go.
func copyBlob(b *windows.DataBlob) []byte {
	out := make([]byte, b.Size)
	copy(out, unsafe.Slice(b.Data, b.Size))
	return out
}

func freeBlob(b *windows.DataBlob) {
	if b.Data != nil {
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(b.Data)))
	}
}
