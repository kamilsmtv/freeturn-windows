package tray

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procPostMessage      = user32.NewProc("PostMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procAppendMenu       = user32.NewProc("AppendMenuW")
	procModifyMenu       = user32.NewProc("ModifyMenuW")
	procEnableMenuItem   = user32.NewProc("EnableMenuItem")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procLoadIcon         = user32.NewProc("LoadIconW")
	procCreateIconFromEx = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon      = user32.NewProc("DestroyIcon")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")

	procShellNotifyIcon = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
)

// Константы Win32, которых нет в x/sys/windows.
const (
	wmDestroy     = 0x0002
	wmCommand     = 0x0111
	wmLButtonUp   = 0x0202
	wmRButtonUp   = 0x0205
	wmLButtonDown = 0x0201

	// Свои сообщения: клик по значку и запрос на обновление подписи.
	wmTrayIcon = 0x0400 + 1 // WM_APP+1
	wmUpdate   = 0x0400 + 2 // WM_APP+2

	wmSettingChange = 0x001A

	nimAdd    = 0x0000
	nimModify = 0x0001
	nimDelete = 0x0002

	nifMessage = 0x0001
	nifIcon    = 0x0002
	nifTip     = 0x0004

	mfString  = 0x0000
	mfGrayed  = 0x0001
	mfEnabled = 0x0000
	mfByCmd   = 0x0000

	tpmRightButton = 0x0002
	tpmLeftAlign   = 0x0000

	lrDefaultColor = 0x0000

	idiApplication = 32512

	smCXSmIcon = 49
	smCYSmIcon = 50

	// Версия формата иконки для CreateIconFromResourceEx.
	iconVersion = 0x00030000
)

// Идентификаторы пунктов меню.
const (
	idStatus = 1000 + iota
	idShow
	idConnect
	idDisconnect
	idQuit
)

type point struct{ X, Y int32 }

type msg struct {
	HWnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

// notifyIconData - NOTIFYICONDATAW. Поля идут в порядке структуры Win32,
// выравнивание Go совпадает с MSVC, размер берём через unsafe.Sizeof.
type notifyIconData struct {
	CbSize           uint32
	HWnd             windows.HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

type impl struct {
	hwnd windows.HWND
	menu uintptr
	icon windows.Handle
	// iconOwned - иконку создали мы и обязаны освободить; системная из
	// LoadIcon освобождения не требует.
	iconOwned bool

	mu    sync.Mutex
	state State
}

func (t *Tray) start() {
	t.impl = &impl{}
	go t.loop()
}

// loop живёт на своём потоке ОС: цикл сообщений Win32 привязан к потоку,
// а горутину планировщик Go может переносить между потоками.
func (t *Tray) loop() {
	lockThread()
	defer unlockThread()

	if err := t.createWindow(); err != nil {
		return
	}
	defer t.removeIcon()

	t.createMenu()
	if err := t.addIcon(); err != nil {
		return
	}
	t.applyState()

	var m msg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		// 0 - WM_QUIT, -1 - ошибка.
		if int32(r) <= 0 {
			return
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (t *Tray) createWindow() error {
	instance, _, _ := procGetModuleHandle.Call(0)
	className, err := windows.UTF16PtrFromString("FreeTurnTrayWindow")
	if err != nil {
		return err
	}

	class := wndClassEx{
		Style:     0,
		WndProc:   windows.NewCallback(t.wndProc),
		Instance:  windows.Handle(instance),
		ClassName: className,
	}
	class.Size = uint32(unsafe.Sizeof(class))
	if atom, _, _ := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return windows.GetLastError()
	}

	title, _ := windows.UTF16PtrFromString("FreeTurn")
	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0,
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		return windows.GetLastError()
	}
	t.impl.hwnd = windows.HWND(hwnd)
	t.applyIcon(t.currentState())
	return nil
}

// systemIcon - запасной вариант, если своя иконка не создалась.
func systemIcon() windows.Handle {
	h, _, _ := procLoadIcon.Call(0, idiApplication)
	return windows.Handle(h)
}

// createIcon делает HICON из содержимого .ico в памяти.
//
// Файл .ico - каталог из нескольких изображений; подходящее система сама
// не выберет, поэтому запись нужного размера ищем сами и отдаём её
// содержимое в CreateIconFromResourceEx.
func createIcon(ico []byte, want int32) (windows.Handle, bool) {
	entry, ok := pickIconEntry(ico, want)
	if !ok {
		return 0, false
	}
	h, _, _ := procCreateIconFromEx.Call(
		uintptr(unsafe.Pointer(&entry[0])),
		uintptr(len(entry)),
		1, // fIcon: иконка, не курсор
		iconVersion,
		uintptr(want), uintptr(want),
		lrDefaultColor,
	)
	if h == 0 {
		return 0, false
	}
	return windows.Handle(h), true
}

// darkTaskbar сообщает, тёмная ли панель задач: от этого зависит, каким
// должен быть серый значок состояния «отключено».
//
// Значение живёт в реестре пользователя; на ошибку отвечаем «тёмная» -
// это тема Windows по умолчанию, и светлый глиф на светлой панели виден
// лучше, чем тёмный на тёмной.
func darkTaskbar() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer func() { _ = key.Close() }()

	light, _, err := key.GetIntegerValue("SystemUsesLightTheme")
	if err != nil {
		return true
	}
	return light == 0
}

// smallIconSize - размер значка в трее с учётом масштабирования экрана.
func smallIconSize() int32 {
	w, _, _ := procGetSystemMetrics.Call(smCXSmIcon)
	if w == 0 {
		return 16
	}
	return int32(w)
}

// applyIcon ставит значку иконку текущего состояния и освобождает прежнюю.
func (t *Tray) applyIcon(s State) {
	i := t.impl
	handle, ok := createIcon(iconForState(s, darkTaskbar()), smallIconSize())
	if !ok {
		if i.icon == 0 {
			i.icon, i.iconOwned = systemIcon(), false
		}
		// Иначе оставляем прежнюю: пустой значок хуже неточного.
		return
	}

	prev, prevOwned := i.icon, i.iconOwned
	i.icon, i.iconOwned = handle, true

	if i.hwnd != 0 {
		data := t.iconData(nifIcon)
		_, _, _ = procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&data)))
	}
	if prevOwned && prev != 0 {
		_, _, _ = procDestroyIcon.Call(uintptr(prev))
	}
}

func (t *Tray) createMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	t.impl.menu = menu

	appendItem(menu, mfString|mfGrayed, idStatus, "Отключено")
	appendSeparator(menu)
	appendItem(menu, mfString, idShow, "Показать окно")
	appendItem(menu, mfString, idConnect, "Подключить")
	appendItem(menu, mfString|mfGrayed, idDisconnect, "Отключить")
	appendSeparator(menu)
	appendItem(menu, mfString, idQuit, "Выход")
}

func (t *Tray) addIcon() error {
	data := t.iconData(nifMessage | nifIcon | nifTip)
	setTip(&data, "FreeTurn")
	if r, _, _ := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&data))); r == 0 {
		return windows.GetLastError()
	}
	return nil
}

func (t *Tray) removeIcon() {
	data := t.iconData(0)
	_, _, _ = procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
	if t.impl.menu != 0 {
		_, _, _ = procDestroyMenu.Call(t.impl.menu)
	}
	if t.impl.hwnd != 0 {
		_, _, _ = procDestroyWindow.Call(uintptr(t.impl.hwnd))
	}
	if t.impl.iconOwned && t.impl.icon != 0 {
		_, _, _ = procDestroyIcon.Call(uintptr(t.impl.icon))
		t.impl.icon, t.impl.iconOwned = 0, false
	}
}

func (t *Tray) iconData(flags uint32) notifyIconData {
	d := notifyIconData{
		HWnd:             t.impl.hwnd,
		UID:              1,
		UFlags:           flags,
		UCallbackMessage: wmTrayIcon,
		HIcon:            t.impl.icon,
	}
	d.CbSize = uint32(unsafe.Sizeof(d))
	return d
}

// wndProc обрабатывает клики по значку и выбор пунктов меню.
func (t *Tray) wndProc(hwnd windows.HWND, message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case wmTrayIcon:
		switch uint32(lparam) {
		case wmLButtonUp:
			// Левая кнопка сразу открывает окно - без меню.
			t.dispatch(t.handlers.Show)
		case wmRButtonUp:
			t.showMenu()
		}
		return 0

	case wmCommand:
		switch uint32(wparam) & 0xFFFF {
		case idShow:
			t.dispatch(t.handlers.Show)
		case idConnect:
			t.dispatch(t.handlers.Connect)
		case idDisconnect:
			t.dispatch(t.handlers.Disconnect)
		case idQuit:
			t.dispatch(t.handlers.Quit)
		}
		return 0

	case wmUpdate:
		t.applyState()
		return 0

	case wmSettingChange:
		// Смена темы Windows приходит сюда: значок «отключено» надо
		// перерисовать в цвет, который будет виден на новой панели.
		t.applyIcon(t.currentState())
		return 0

	case wmDestroy:
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	}

	r, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wparam, lparam)
	return r
}

// dispatch выполняет действие в отдельной горутине: обработчик может
// подождать окно или остановку ядра, а цикл сообщений останавливать нельзя -
// иначе значок перестаёт отвечать на клики.
func (t *Tray) dispatch(f func()) {
	if f == nil {
		return
	}
	go f()
}

func (t *Tray) showMenu() {
	var pt point
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// Без переднего плана меню не закроется по клику мимо него - так
	// описано в документации TrackPopupMenu.
	_, _, _ = procSetForeground.Call(uintptr(t.impl.hwnd))
	_, _, _ = procTrackPopupMenu.Call(t.impl.menu, tpmLeftAlign|tpmRightButton,
		uintptr(pt.X), uintptr(pt.Y), 0, uintptr(t.impl.hwnd), 0)
	_, _, _ = procPostMessage.Call(uintptr(t.impl.hwnd), 0, 0, 0)
}

func (t *Tray) setState(s State) {
	if t.impl == nil {
		return
	}
	t.impl.mu.Lock()
	t.impl.state = s
	t.impl.mu.Unlock()

	// Значок и меню трогаем только на потоке цикла сообщений.
	if t.impl.hwnd != 0 {
		_, _, _ = procPostMessage.Call(uintptr(t.impl.hwnd), wmUpdate, 0, 0)
	}
}

// currentState отдаёт состояние под замком: его пишет вызывающий код, а
// читает поток цикла сообщений.
func (t *Tray) currentState() State {
	t.impl.mu.Lock()
	defer t.impl.mu.Unlock()
	return t.impl.state
}

// applyState вызывается на потоке цикла сообщений.
func (t *Tray) applyState() {
	i := t.impl
	s := t.currentState()

	if i.menu == 0 || i.hwnd == 0 {
		return
	}

	title := "Отключено"
	if s.Connected {
		title = "Подключено"
	}
	if s.Profile != "" {
		title += ": " + s.Profile
	}
	modifyItem(i.menu, idStatus, mfString|mfGrayed, title)

	enable(i.menu, idConnect, !s.Connected)
	enable(i.menu, idDisconnect, s.Connected)

	t.applyIcon(s)

	tip := "FreeTurn - " + title
	if s.Detail != "" {
		tip += " (" + s.Detail + ")"
	}
	data := t.iconData(nifTip)
	setTip(&data, tip)
	_, _, _ = procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&data)))
}

func (t *Tray) stop() {
	if t.impl == nil || t.impl.hwnd == 0 {
		return
	}
	// Цикл сообщений сам снимет значок и выйдет.
	_, _, _ = procPostMessage.Call(uintptr(t.impl.hwnd), wmDestroy, 0, 0)
}

func appendItem(menu uintptr, flags uint32, id int, text string) {
	ptr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	_, _, _ = procAppendMenu.Call(menu, uintptr(flags), uintptr(id), uintptr(unsafe.Pointer(ptr)))
}

func appendSeparator(menu uintptr) {
	const mfSeparator = 0x0800
	_, _, _ = procAppendMenu.Call(menu, mfSeparator, 0, 0)
}

func modifyItem(menu uintptr, id int, flags uint32, text string) {
	ptr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	_, _, _ = procModifyMenu.Call(menu, uintptr(id), uintptr(flags|mfByCmd), uintptr(id), uintptr(unsafe.Pointer(ptr)))
}

func enable(menu uintptr, id int, on bool) {
	flags := uint32(mfByCmd | mfGrayed)
	if on {
		flags = mfByCmd | mfEnabled
	}
	_, _, _ = procEnableMenuItem.Call(menu, uintptr(id), uintptr(flags))
}

// setTip копирует подсказку в фиксированный буфер структуры.
func setTip(d *notifyIconData, tip string) {
	utf16 := windows.StringToUTF16(tip)
	if len(utf16) > len(d.SzTip) {
		utf16 = utf16[:len(d.SzTip)-1]
		utf16 = append(utf16, 0)
	}
	copy(d.SzTip[:], utf16)
}
