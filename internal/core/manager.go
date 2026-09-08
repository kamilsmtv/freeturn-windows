package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/errs"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// State - состояние ядра.
type State string

// Состояния, которые видит UI.
const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateFailed   State = "failed"
)

// LogLine - строка журнала ядра.
type LogLine struct {
	Seq   int64  `json:"seq"`
	Time  string `json:"time"`
	Level string `json:"level"`
	Text  string `json:"text"`
}

// Status - снимок состояния для UI.
type Status struct {
	State     State  `json:"state"`
	ProfileID string `json:"profileId"`
	PID       int    `json:"pid"`
	Version   string `json:"version"`
	StartedAt string `json:"startedAt"`
	Error     string `json:"error"`
}

// Manager запускает ядро, транслирует его журнал и следит за завершением.
//
// Одновременно живёт не более одного процесса ядра: подключение к двум
// профилям сразу всё равно упёрлось бы в один и тот же локальный порт.
type Manager struct {
	exePath func() (string, error)

	mu      sync.RWMutex
	state   State
	profID  string
	version string
	started time.Time
	lastErr string
	cmd     *exec.Cmd
	job     *jobObject
	cancel  context.CancelFunc
	done    chan struct{}

	logMu  sync.RWMutex
	log    []LogLine
	logSeq int64
	// sessionSeq - номер последней строки перед текущим запуском ядра.
	sessionSeq int64
	logCap     int
	logSink    func(LogLine)

	onState func(Status)
}

// NewManager создаёт менеджер. exePath вызывается при каждом запуске, чтобы
// подхватывать бинарь, только что заменённый апдейтером.
func NewManager(exePath func() (string, error), logCap int) *Manager {
	if logCap < 100 {
		logCap = 5000
	}
	return &Manager{exePath: exePath, state: StateStopped, logCap: logCap}
}

// OnState подписывает UI на смену состояния.
func (m *Manager) OnState(f func(Status)) { m.onState = f }

// OnLog подписывает UI на новые строки журнала.
func (m *Manager) OnLog(f func(LogLine)) { m.logSink = f }

// Status возвращает текущий снимок состояния.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	st := Status{State: m.state, ProfileID: m.profID, Version: m.version, Error: m.lastErr}
	if m.cmd != nil && m.cmd.Process != nil {
		st.PID = m.cmd.Process.Pid
	}
	if !m.started.IsZero() {
		st.StartedAt = m.started.Format(time.RFC3339)
	}
	return st
}

// Running сообщает, работает ли ядро прямо сейчас.
func (m *Manager) Running() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state == StateStarting || m.state == StateRunning
}

// ErrAlreadyRunning - попытка запустить второй экземпляр ядра.
var ErrAlreadyRunning = errors.New("ядро уже запущено")

// Start проверяет профиль, запускает ядро и начинает читать его журнал.
func (m *Manager) Start(p profile.Profile) error {
	if m.Running() {
		return ErrAlreadyRunning
	}
	if err := Validate(p); err != nil {
		return err
	}
	exe, err := m.exePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(exe); err != nil {
		return errors.New("бинарь ядра не найден - скачайте его на вкладке обновлений")
	}

	m.markSession()

	args := BuildArgs(p)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, exe, args...)
	hideWindow(cmd)

	// Ядро пишет журнал через stdlib log, то есть в stderr; stdout остаётся
	// для сообщений вроде ссылки на ручную captcha.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}

	m.appendLog("info", "Запуск ядра: "+redactArgs(args))

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("не удалось запустить ядро: %w", err)
	}

	job, jerr := newJob()
	if jerr == nil {
		if err := job.assign(cmd.Process); err != nil {
			m.appendLog("warn", "не удалось привязать ядро к job-объекту: "+err.Error())
		}
	}

	m.mu.Lock()
	m.state, m.profID, m.cmd, m.cancel, m.job = StateStarting, p.ID, cmd, cancel, job
	m.started, m.lastErr, m.version = time.Now(), "", ""
	m.done = make(chan struct{})
	done := m.done
	m.mu.Unlock()
	m.emitState()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); m.pump(stderr) }()
	go func() { defer wg.Done(); m.pump(stdout) }()

	go m.wait(cmd, &wg, done)
	return nil
}

// Stop просит ядро завершиться и ждёт, пока процесс действительно исчезнет.
func (m *Manager) Stop() error {
	m.mu.Lock()
	cmd, cancel, done := m.cmd, m.cancel, m.done
	if cmd == nil || cmd.Process == nil {
		m.mu.Unlock()
		return nil
	}
	m.state = StateStopping
	m.mu.Unlock()
	m.emitState()
	m.appendLog("info", "Остановка ядра")

	// CommandContext убивает процесс; сигналов, которые Windows доставила бы
	// чужой консоли, у нас нет, а ядро при выходе само снимает свои маршруты.
	if cancel != nil {
		cancel()
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
	}
	return nil
}

// Close останавливает ядро и освобождает job-объект. Вызывается при выходе.
func (m *Manager) Close() {
	_ = m.Stop()
	m.mu.Lock()
	job := m.job
	m.job = nil
	m.mu.Unlock()
	job.close()
}

// wait дожидается завершения процесса и переводит менеджер в конечное состояние.
func (m *Manager) wait(cmd *exec.Cmd, wg *sync.WaitGroup, done chan struct{}) {
	waitErr := cmd.Wait()
	wg.Wait() // журнал дочитан до конца, включая последнюю строку об ошибке

	m.mu.Lock()
	stopping := m.state == StateStopping
	m.cmd, m.cancel = nil, nil
	switch {
	case stopping || waitErr == nil:
		m.state, m.lastErr = StateStopped, ""
	default:
		m.state = StateFailed
		m.lastErr = errs.Explain(m.recentLines(), waitErr)
	}
	job := m.job
	m.job = nil
	m.mu.Unlock()

	job.close()
	close(done)

	if st := m.Status(); st.State == StateFailed {
		m.appendLog("error", "Ядро остановлено: "+st.Error)
	}
	m.emitState()
}

// pump читает поток ядра построчно и складывает в журнал.
func (m *Manager) pump(r io.Reader) {
	scanner := bufio.NewScanner(r)
	// Строки с captcha-URL длиннее стандартных 64 КиБ не бывают, но запас берём.
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if v := ParseVersionLine(line); v != "" {
			m.mu.Lock()
			m.version = v
			if m.state == StateStarting {
				m.state = StateRunning
			}
			m.mu.Unlock()
			m.emitState()
		}
		m.appendLog(levelOf(line), line)
	}
}

// HasLogLine сообщает, встречалась ли строка с подстрокой sub в журнале
// текущего запуска.
//
// Искать по всему журналу нельзя: он переживает остановку ядра, и признак
// готовности от прошлого сеанса сработал бы сразу после нового запуска.
func (m *Manager) HasLogLine(sub string) bool {
	m.logMu.RLock()
	defer m.logMu.RUnlock()
	for _, l := range m.log {
		if l.Seq > m.sessionSeq && strings.Contains(l.Text, sub) {
			return true
		}
	}
	return false
}

// markSession отмечает границу журнала, с которой начинается новый запуск.
func (m *Manager) markSession() {
	m.logMu.Lock()
	m.sessionSeq = m.logSeq
	m.logMu.Unlock()
}

// Log отдаёт накопленный журнал (для первичной отрисовки вкладки).
func (m *Manager) Log() []LogLine {
	m.logMu.RLock()
	defer m.logMu.RUnlock()
	out := make([]LogLine, len(m.log))
	copy(out, m.log)
	return out
}

// ClearLog очищает журнал.
func (m *Manager) ClearLog() {
	m.logMu.Lock()
	m.log = nil
	m.logMu.Unlock()
}

// AppendLog добавляет в журнал строку от самого приложения.
func (m *Manager) AppendLog(level, text string) { m.appendLog(level, text) }

func (m *Manager) appendLog(level, text string) {
	m.logMu.Lock()
	m.logSeq++
	line := LogLine{Seq: m.logSeq, Time: time.Now().Format("15:04:05"), Level: level, Text: text}
	m.log = append(m.log, line)
	if len(m.log) > m.logCap {
		// Режем с запасом, чтобы не копировать срез на каждой строке.
		m.log = append([]LogLine(nil), m.log[len(m.log)-m.logCap:]...)
	}
	sink := m.logSink
	m.logMu.Unlock()

	if sink != nil {
		sink(line)
	}
}

// recentLines возвращает хвост журнала для разбора причины падения.
func (m *Manager) recentLines() []string {
	m.logMu.RLock()
	defer m.logMu.RUnlock()
	const tail = 40
	from := len(m.log) - tail
	if from < 0 {
		from = 0
	}
	out := make([]string, 0, len(m.log)-from)
	for _, l := range m.log[from:] {
		out = append(out, l.Text)
	}
	return out
}

func (m *Manager) emitState() {
	if m.onState != nil {
		m.onState(m.Status())
	}
}

// levelOf определяет уровень по префиксу, который печатает logx ядра.
func levelOf(line string) string {
	switch {
	case containsTag(line, "[ERROR]"):
		return "error"
	case containsTag(line, "[WARN]"):
		return "warn"
	case containsTag(line, "[DEBUG]"):
		return "debug"
	default:
		return "info"
	}
}

func containsTag(line, tag string) bool {
	// Теги стоят в начале сообщения, после метки времени stdlib log.
	if len(line) < len(tag) {
		return false
	}
	for i := 0; i+len(tag) <= len(line) && i < 64; i++ {
		if line[i:i+len(tag)] == tag {
			return true
		}
	}
	return false
}

// redactArgs прячет ключ обфускации в журнале: он же лежит в скриншотах.
func redactArgs(args []string) string {
	out := make([]string, len(args))
	copy(out, args)
	for i := 0; i+1 < len(out); i++ {
		if out[i] == "-obf-key" {
			out[i+1] = "<скрыт>"
		}
	}
	return joinArgs(out)
}

func joinArgs(args []string) string {
	s := ""
	for i, a := range args {
		if i > 0 {
			s += " "
		}
		s += a
	}
	return s
}
