// Package tray показывает значок FreeTurn в области уведомлений Windows.
//
// В Wails v2 своего трея нет, поэтому значок ведёт отдельная библиотека со
// своим циклом сообщений; она живёт в собственной горутине рядом с окном.
package tray

// Handlers - действия, которые вызывает меню значка.
type Handlers struct {
	Show       func()
	Connect    func()
	Disconnect func()
	Quit       func()
}

// State - что показывать в значке.
type State struct {
	Connected bool
	// Failed - ядро остановилось с ошибкой; значок краснеет.
	Failed  bool
	Profile string
	// Detail - строка под названием профиля (ошибка или состояние).
	Detail string
}

// Tray - значок в области уведомлений.
type Tray struct {
	handlers Handlers
	impl     *impl
}

// New создаёт значок. Возвращает управляемый объект даже там, где трея нет.
func New(h Handlers) *Tray { return &Tray{handlers: h} }

// Start запускает значок. Вызов не блокирующий.
func (t *Tray) Start() { t.start() }

// SetState обновляет подпись и пункты меню.
func (t *Tray) SetState(s State) { t.setState(s) }

// Stop убирает значок.
func (t *Tray) Stop() { t.stop() }
