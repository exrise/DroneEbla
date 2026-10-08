// Package netplay — сетевая игра (хост-авторитарная модель) и локальные
// сессии. Хост считает всю симуляцию и отправляет клиенту только то, что
// клиент видит; клиент отправляет приказы.
package netplay

import (
	"compress/flate"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/exrise/droneebla/internal/ai"
	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
	"github.com/exrise/droneebla/internal/world"
)

// Version — версия протокола.
const Version = 5

// DefaultPort — порт по умолчанию.
const DefaultPort = 27015

// Session — то, с чем работает интерфейс.
type Session interface {
	View() *sim.View
	Send(c sim.Command)
	Messages() []string
	Side() int
	SetSide(s int)      // только песочница
	Lobby() *LobbyState // лобби сетевой игры (nil — не сетевая)
	PickSide(side int)  // выбрать сторону в лобби
	StartGame()         // начать партию (только хост)
	IsHost() bool
	Sandbox() bool
	Status() string // "" — идёт игра
	Save(path string) error
	Close()
}

// Hello — первое сообщение клиента.
type Hello struct {
	Version  int
	DataHash string
}

// MaxPerSide — сколько игроков может играть за одну сторону; всего до 6.
const MaxPerSide = 3

// Welcome — ответ хоста.
type Welcome struct {
	ID    int // номер игрока (0 — хост)
	Error string
}

// Pick — выбор стороны клиентом.
type Pick struct{ Side int }

// LobbyPlayer — игрок в лобби.
type LobbyPlayer struct {
	ID   int
	Name string
	Side int // -1 — сторона ещё не выбрана
	Host bool
}

// LobbyState — состояние лобби, которое хост рассылает всем игрокам.
type LobbyState struct {
	You     int // номер получателя
	Players []LobbyPlayer
	Started bool
	Max     int
}

// Count — сколько игроков на стороне side.
func (l *LobbyState) Count(side int) int {
	n := 0
	for _, p := range l.Players {
		if p.Side == side {
			n++
		}
	}
	return n
}

// Me — запись получателя.
func (l *LobbyState) Me() LobbyPlayer {
	for _, p := range l.Players {
		if p.ID == l.You {
			return p
		}
	}
	return LobbyPlayer{ID: l.You, Side: -1}
}

// Msg — конверт сетевого сообщения.
type Msg struct {
	Hello   *Hello
	Welcome *Welcome
	Cmd     *sim.Command
	View    *sim.View
	Notice  string
	Lobby   *LobbyState
	Pick    *Pick
}

// conn — gob поверх сжатого потока.
type conn struct {
	c   net.Conn
	fw  *flate.Writer
	enc *gob.Encoder
	dec *gob.Decoder
	mu  sync.Mutex
}

func newConn(c net.Conn) *conn {
	fw, _ := flate.NewWriter(c, flate.BestSpeed)
	return &conn{c: c, fw: fw, enc: gob.NewEncoder(fw), dec: gob.NewDecoder(flate.NewReader(c))}
}

func (k *conn) send(m *Msg) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := k.enc.Encode(m); err != nil {
		return err
	}
	return k.fw.Flush()
}

func (k *conn) recv() (*Msg, error) {
	var m Msg
	err := k.dec.Decode(&m)
	return &m, err
}

// DataHash — отпечаток игровых данных (у обоих игроков должен совпадать).
func DataHash(override string) string {
	h := sha256.New()
	for _, f := range data.Files {
		b, err := os.ReadFile(filepath.Join(override, f))
		if err != nil || override == "" {
			b, _ = data.DefaultFile(f)
		}
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// LocalIPs — адреса этого компьютера (адрес Radmin VPN обычно 26.x.x.x).
func LocalIPs() []string {
	var out []string
	ifs, _ := net.InterfaceAddrs()
	for _, a := range ifs {
		if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() {
			s := ip.IP.String()
			out = append(out, s)
		}
	}
	sort.Slice(out, func(a, b int) bool {
		return strings.HasPrefix(out[a], "26.") && !strings.HasPrefix(out[b], "26.")
	})
	return out
}

// ---------------------------------------------------------------------
// Хост и песочница.

// Host — локальная симуляция; при сетевой игре принимает до 5 клиентов (до 3 игроков на сторону).
type Host struct {
	mu       sync.Mutex
	w        *sim.World
	side     int // сторона хоста
	sandbox  bool
	ln       net.Listener
	players  []*player
	nextID   int
	started  bool // сетевая партия началась (до этого — лобби)
	network  bool
	msgs     []string
	stop     chan struct{}
	view     *sim.View
	viewAt   time.Time
	dataHash string
	ai       *ai.AI // компьютерный противник (одиночная игра)
}

// player — подключённый клиент.
type player struct {
	id      int
	k       *conn
	side    int // -1 — не выбрана
	sending bool
	lastEv  uint64
}

// LogDir — папка журналов партий; пусто — журнал не ведётся (например, в тестах).
var LogDir string

// startLog открывает журнал партии: <LogDir>/<время>_<режим>.jsonl.
func startLog(w *sim.World, mode string, extra map[string]any) {
	if LogDir == "" {
		return
	}
	if err := os.MkdirAll(LogDir, 0o755); err != nil {
		return
	}
	name := time.Now().Format("2006-01-02_15-04-05") + "_" + mode + ".jsonl"
	f, err := os.Create(filepath.Join(LogDir, name))
	if err != nil {
		return
	}
	if extra == nil {
		extra = map[string]any{}
	}
	extra["protocol"] = Version
	w.SetRecorder(f, mode, extra)
}

// NewSandbox — одиночная игра без сети.
func NewSandbox(w *sim.World, side int) *Host {
	startLog(w, "sandbox", map[string]any{"side": side})
	w.NetHost = false
	h := &Host{w: w, side: side, sandbox: true, stop: make(chan struct{})}
	go h.loop()
	return h
}

// NewSolo — одиночная игра: человек играет за сторону human, другой стороной
// управляет ИИ. Правила и туман войны те же, что в сетевой игре.
func NewSolo(w *sim.World, human int) *Host {
	w.Sandbox, w.Solo, w.Human, w.NetHost = true, true, human, false
	w.StartPlacement()
	startLog(w, "solo", map[string]any{"human": human})
	h := &Host{w: w, side: human, sandbox: true, ai: ai.New(w.Catalog(), 1-human), stop: make(chan struct{})}
	go h.loop()
	return h
}

// NewHost — сетевая игра: ждёт клиента на порту.
func NewHost(w *sim.World, side int, port int, dataHash string) (*Host, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	w.StartPlacement()
	w.NetHost, w.HostSide = true, side
	startLog(w, "network", map[string]any{"host_side": side, "data_hash": dataHash})
	h := &Host{w: w, side: side, ln: ln, stop: make(chan struct{}), dataHash: dataHash, network: true, nextID: 1}
	go h.accept()
	go h.loop()
	return h, nil
}

func (h *Host) accept() {
	for {
		c, err := h.ln.Accept()
		if err != nil {
			return
		}
		k := newConn(c)
		c.SetReadDeadline(time.Now().Add(10 * time.Second))
		m, err := k.recv()
		c.SetReadDeadline(time.Time{})
		if err != nil || m.Hello == nil {
			c.Close()
			continue
		}
		if m.Hello.Version != Version {
			k.send(&Msg{Welcome: &Welcome{Error: "разные версии игры"}})
			c.Close()
			continue
		}
		if m.Hello.DataHash != h.dataHash {
			k.send(&Msg{Welcome: &Welcome{Error: "игровые данные (папка data) у игроков различаются"}})
			c.Close()
			continue
		}
		h.mu.Lock()
		if len(h.players) >= 2*MaxPerSide-1 {
			h.mu.Unlock()
			k.send(&Msg{Welcome: &Welcome{Error: "в игре уже 6 игроков"}})
			c.Close()
			continue
		}
		p := &player{id: h.nextID, k: k, side: -1}
		h.nextID++
		h.players = append(h.players, p)
		h.note(fmt.Sprintf("Игрок %d подключился: %s", p.id, c.RemoteAddr().String()))
		h.msgs = append(h.msgs, fmt.Sprintf("Игрок %d подключился", p.id))
		ls := h.lobbyFor(p.id)
		h.mu.Unlock()
		k.send(&Msg{Welcome: &Welcome{ID: p.id}})
		k.send(&Msg{Lobby: ls})
		h.broadcastLobby(p.id)
		go h.readPlayer(p)
	}
}

// note пишет заметку в журнал партии (под блокировкой хоста).
func (h *Host) note(text string) { h.w.RecordNote(text) }

// humans — сколько живых игроков на стороне (хост тоже считается).
func (h *Host) humans(side int) int {
	n := 0
	if h.side == side {
		n++
	}
	for _, p := range h.players {
		if p.side == side {
			n++
		}
	}
	return n
}

// lobbyFor — состояние лобби для игрока id.
func (h *Host) lobbyFor(id int) *LobbyState {
	ls := &LobbyState{You: id, Started: h.started, Max: MaxPerSide}
	ls.Players = append(ls.Players, LobbyPlayer{ID: 0, Name: "Хост", Side: h.side, Host: true})
	for _, p := range h.players {
		ls.Players = append(ls.Players, LobbyPlayer{ID: p.id, Name: fmt.Sprintf("Игрок %d", p.id), Side: p.side})
	}
	return ls
}

// broadcastLobby рассылает лобби всем клиентам, кроме except (−1 — всем).
func (h *Host) broadcastLobby(except int) {
	h.mu.Lock()
	type out struct {
		k  *conn
		ls *LobbyState
	}
	var outs []out
	for _, p := range h.players {
		if p.id != except {
			outs = append(outs, out{p.k, h.lobbyFor(p.id)})
		}
	}
	h.mu.Unlock()
	for _, o := range outs {
		o.k.send(&Msg{Lobby: o.ls})
	}
}

// pick — выбор стороны игроком (под блокировкой).
func (h *Host) pick(p *player, side int) string {
	if side < 0 || side > 1 {
		return "Неверная сторона"
	}
	if p.side == side {
		return ""
	}
	if h.started && p.side >= 0 {
		return "После начала партии сторону менять нельзя"
	}
	if h.humans(side) >= MaxPerSide {
		return fmt.Sprintf("За %s уже %d игрока", data.SideNames[side], MaxPerSide)
	}
	p.side = side
	h.note(fmt.Sprintf("Игрок %d выбрал сторону: %s", p.id, data.SideNames[side]))
	return ""
}

func (h *Host) readPlayer(p *player) {
	for {
		m, err := p.k.recv()
		if err != nil {
			h.mu.Lock()
			for i, q := range h.players {
				if q == p {
					h.players = append(h.players[:i], h.players[i+1:]...)
					break
				}
			}
			h.note(fmt.Sprintf("Игрок %d отключился", p.id))
			h.msgs = append(h.msgs, fmt.Sprintf("Игрок %d отключился", p.id))
			h.mu.Unlock()
			p.k.c.Close()
			h.broadcastLobby(-1)
			return
		}
		if m.Pick != nil {
			h.mu.Lock()
			e := h.pick(p, m.Pick.Side)
			h.mu.Unlock()
			if e != "" {
				p.k.send(&Msg{Notice: e})
			}
			h.broadcastLobby(-1)
			continue
		}
		if m.Cmd != nil {
			h.mu.Lock()
			c := *m.Cmd
			e := ""
			switch {
			case p.side < 0 || !h.started:
				e = "Сначала выберите сторону и дождитесь начала игры"
			case c.Kind == sim.CmdSpeed || c.Kind == sim.CmdPause:
				e = "Скоростью и паузой управляет хост"
			default:
				c.Side, c.Player = p.side, p.id
				e = h.w.Apply(c)
			}
			h.mu.Unlock()
			if e != "" {
				p.k.send(&Msg{Notice: e})
			}
		}
	}
}

// running — идёт ли время: партия началась и у каждой стороны есть живой игрок.
func (h *Host) running() bool {
	if h.sandbox {
		return true
	}
	return h.started && h.humans(0) > 0 && h.humans(1) > 0
}

type outView struct {
	p *player
	v *sim.View
}

func (h *Host) loop() {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	last := time.Now()
	sendAcc := 0.0
	for {
		select {
		case <-h.stop:
			return
		case now := <-t.C:
			dt := now.Sub(last).Seconds()
			last = now
			if dt > 0.5 {
				dt = 0.5
			}
			h.mu.Lock()
			if h.running() {
				h.w.Update(dt)
				if h.ai != nil && (!h.w.Paused() || h.w.Placement) {
					h.ai.Tick(h.w)
				}
			}
			var outs []outView
			sendAcc += dt
			if h.started && sendAcc >= 0.2 {
				sendAcc = 0
				outs = h.collectViews()
			}
			h.mu.Unlock()
			for _, o := range outs {
				go func(o outView) {
					err := o.p.k.send(&Msg{View: o.v})
					h.mu.Lock()
					o.p.sending = false
					if err == nil {
						if n := len(o.v.Events); n > 0 && o.v.Events[n-1].ID > o.p.lastEv {
							o.p.lastEv = o.v.Events[n-1].ID
						}
					}
					h.mu.Unlock()
					if err != nil {
						o.p.k.c.Close()
					}
				}(o)
			}
		}
	}
}

// collectViews строит представления для клиентов (под блокировкой): одно на сторону,
// с общим туманом войны; события фильтруются по курсору каждого игрока.
func (h *Host) collectViews() []outView {
	var outs []outView
	for side := 0; side < 2; side++ {
		var due []*player
		minEv := ^uint64(0)
		for _, p := range h.players {
			if p.side == side && !p.sending {
				due = append(due, p)
				if p.lastEv < minEv {
					minEv = p.lastEv
				}
			}
		}
		if len(due) == 0 {
			continue
		}
		base := h.w.BuildView(side, minEv)
		for _, p := range due {
			vv := *base
			vv.Events = nil
			for _, e := range base.Events {
				if e.ID > p.lastEv {
					vv.Events = append(vv.Events, e)
				}
			}
			// Скоростью и паузой управляет хост: клиент видит их, но не меняет.
			vv.TimeLocked = true
			vv.MySpeed = vv.Speed
			vv.Pausing = vv.Paused
			p.sending = true
			outs = append(outs, outView{p, &vv})
		}
	}
	return outs
}

// View — представление для локального игрока.
func (h *Host) View() *sim.View {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.view != nil && time.Since(h.viewAt) < 90*time.Millisecond && h.view.Side == h.side {
		return h.view
	}
	h.view = h.w.BuildView(h.side, 0)
	h.viewAt = time.Now()
	return h.view
}

// Send — приказ локального игрока.
func (h *Host) Send(c sim.Command) {
	h.mu.Lock()
	c.Side = h.side
	e := h.w.Apply(c)
	h.view = nil
	h.mu.Unlock()
	if e != "" {
		h.mu.Lock()
		h.msgs = append(h.msgs, e)
		h.mu.Unlock()
	}
}

// Messages — накопленные сообщения.
func (h *Host) Messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	m := h.msgs
	h.msgs = nil
	return m
}

func (h *Host) Side() int { return h.side }

// Lobby, PickSide и StartGame нужны только сетевой игре.

// SetSide — смена стороны в песочнице.
func (h *Host) SetSide(s int) {
	if h.sandbox && h.ai == nil {
		h.mu.Lock()
		h.side = s
		h.view = nil
		h.mu.Unlock()
	}
}

func (h *Host) IsHost() bool  { return true }
func (h *Host) Sandbox() bool { return h.sandbox }

func (h *Host) Status() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.network && h.started {
		for side := 0; side < 2; side++ {
			if h.humans(side) == 0 {
				return fmt.Sprintf("За сторону «%s» никого нет — пауза до возвращения игроков", data.SideNames[side])
			}
		}
	}
	return ""
}

// Lobby — состояние лобби (nil вне сетевой игры).
func (h *Host) Lobby() *LobbyState {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.network {
		return nil
	}
	return h.lobbyFor(0)
}

// PickSide — хост выбирает свою сторону (только до начала партии).
func (h *Host) PickSide(side int) {
	h.mu.Lock()
	if !h.network || h.started || side < 0 || side > 1 || side == h.side || h.humans(side) >= MaxPerSide {
		h.mu.Unlock()
		return
	}
	h.side = side
	h.w.HostSide = side
	h.view = nil
	h.mu.Unlock()
	h.broadcastLobby(-1)
}

// StartGame — хост начинает партию; нужен хотя бы один игрок за каждую сторону.
func (h *Host) StartGame() {
	h.mu.Lock()
	if !h.network || h.started || h.humans(0) == 0 || h.humans(1) == 0 {
		h.mu.Unlock()
		return
	}
	h.started = true
	h.note("Партия началась")
	h.mu.Unlock()
	h.broadcastLobby(-1)
}

// Save сохраняет партию.
func (h *Host) Save(path string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.w.Save(path)
}

// Close останавливает хост.
func (h *Host) Close() {
	select {
	case <-h.stop:
	default:
		close(h.stop)
	}
	h.mu.Lock()
	h.w.CloseRecorder()
	if h.ln != nil {
		h.ln.Close()
	}
	for _, p := range h.players {
		p.k.c.Close()
	}
	h.mu.Unlock()
}

// World — прямой доступ (для тестов).
func (h *Host) World() *sim.World { return h.w }

// ---------------------------------------------------------------------
// Клиент.

// Client — подключение к хосту.
type Client struct {
	mu     sync.Mutex
	k      *conn
	id     int
	lobby  *LobbyState
	view   *sim.View
	events []sim.Event
	lastEv uint64
	msgs   []string
	status string
	closed bool
}

// Connect подключается к хосту.
func Connect(addr string, dataHash string) (*Client, error) {
	if !strings.Contains(addr, ":") {
		addr = fmt.Sprintf("%s:%d", addr, DefaultPort)
	}
	c, err := net.DialTimeout("tcp", addr, 8*time.Second)
	if err != nil {
		return nil, err
	}
	k := newConn(c)
	if err := k.send(&Msg{Hello: &Hello{Version: Version, DataHash: dataHash}}); err != nil {
		c.Close()
		return nil, err
	}
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	m, err := k.recv()
	c.SetReadDeadline(time.Time{})
	if err != nil {
		c.Close()
		return nil, err
	}
	if m.Welcome == nil {
		c.Close()
		return nil, fmt.Errorf("неожиданный ответ хоста")
	}
	if m.Welcome.Error != "" {
		c.Close()
		return nil, fmt.Errorf("%s", m.Welcome.Error)
	}
	cl := &Client{k: k, id: m.Welcome.ID, status: "Лобби"}
	go cl.read()
	return cl, nil
}

func (cl *Client) read() {
	for {
		m, err := cl.k.recv()
		if err != nil {
			cl.mu.Lock()
			if !cl.closed {
				if err == io.EOF {
					cl.status = "Хост завершил игру"
				} else {
					cl.status = "Связь с хостом потеряна: " + err.Error()
				}
			}
			cl.mu.Unlock()
			return
		}
		cl.mu.Lock()
		if m.View != nil {
			for _, e := range m.View.Events {
				if e.ID > cl.lastEv {
					cl.events = append(cl.events, e)
					cl.lastEv = e.ID
				}
			}
			if len(cl.events) > 300 {
				cl.events = cl.events[len(cl.events)-300:]
			}
			m.View.Events = append([]sim.Event{}, cl.events...)
			cl.view = m.View
			cl.status = ""
		}
		if m.Lobby != nil {
			cl.lobby = m.Lobby
		}
		if m.Notice != "" {
			cl.msgs = append(cl.msgs, m.Notice)
		}
		cl.mu.Unlock()
	}
}

func (cl *Client) View() *sim.View {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.view
}

func (cl *Client) Send(c sim.Command) {
	if cl.Side() < 0 {
		return
	}
	c.Side = cl.Side()
	go func() {
		if err := cl.k.send(&Msg{Cmd: &c}); err != nil {
			cl.mu.Lock()
			cl.msgs = append(cl.msgs, "Не удалось отправить приказ: "+err.Error())
			cl.mu.Unlock()
		}
	}()
}

func (cl *Client) Messages() []string {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	m := cl.msgs
	cl.msgs = nil
	return m
}

func (cl *Client) Side() int {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.lobby == nil {
		return -1
	}
	return cl.lobby.Me().Side
}

// Lobby — последнее состояние лобби от хоста.
func (cl *Client) Lobby() *LobbyState {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.lobby
}

// PickSide просит хост перевести игрока на сторону side.
func (cl *Client) PickSide(side int) {
	go cl.k.send(&Msg{Pick: &Pick{Side: side}})
}

func (cl *Client) StartGame()    {}
func (cl *Client) SetSide(int)   {}
func (cl *Client) IsHost() bool  { return false }
func (cl *Client) Sandbox() bool { return false }
func (cl *Client) Save(string) error {
	return fmt.Errorf("сохранять может только хост")
}

func (cl *Client) Status() string {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.status
}

func (cl *Client) Close() {
	cl.mu.Lock()
	cl.closed = true
	cl.mu.Unlock()
	cl.k.c.Close()
}

// NewWorld — удобная обёртка для создания партии.
func NewWorld(cat *data.Catalog, m *world.MapData, sandbox bool) *sim.World {
	return sim.New(cat, m, sandbox)
}

// Advance — перемотка песочницы на minutes игровых минут (для отладки).
func (h *Host) Advance(minutes float64, f func(w *sim.World)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if f != nil {
		f(h.w)
	}
	for k := 0.0; k < minutes; k++ {
		h.w.Step(1)
		if h.ai != nil {
			h.ai.Tick(h.w)
		}
	}
	h.view = nil
}
