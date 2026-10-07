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
const Version = 3

// DefaultPort — порт по умолчанию.
const DefaultPort = 27015

// Session — то, с чем работает интерфейс.
type Session interface {
	View() *sim.View
	Send(c sim.Command)
	Messages() []string
	Side() int
	SetSide(s int) // только песочница
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

// Welcome — ответ хоста.
type Welcome struct {
	Side  int
	Error string
}

// Msg — конверт сетевого сообщения.
type Msg struct {
	Hello   *Hello
	Welcome *Welcome
	Cmd     *sim.Command
	View    *sim.View
	Notice  string
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

// Host — локальная симуляция; при сетевой игре принимает одного клиента.
type Host struct {
	mu       sync.Mutex
	w        *sim.World
	side     int
	sandbox  bool
	ln       net.Listener
	client   *conn
	status   string
	msgs     []string
	stop     chan struct{}
	lastEv   uint64
	sending  bool
	view     *sim.View
	viewAt   time.Time
	dataHash string
	ai       *ai.AI // компьютерный противник (одиночная игра)
}

// NewSandbox — одиночная игра без сети.
func NewSandbox(w *sim.World, side int) *Host {
	h := &Host{w: w, side: side, sandbox: true, stop: make(chan struct{})}
	go h.loop()
	return h
}

// NewSolo — одиночная игра: человек играет за сторону human, другой стороной
// управляет ИИ. Правила и туман войны те же, что в сетевой игре.
func NewSolo(w *sim.World, human int) *Host {
	w.Sandbox, w.Solo = true, true
	w.StartPlacement()
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
	h := &Host{w: w, side: side, ln: ln, stop: make(chan struct{}), dataHash: dataHash,
		status: fmt.Sprintf("Ожидание второго игрока на порту %d…", port)}
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
		if h.client != nil {
			h.mu.Unlock()
			k.send(&Msg{Welcome: &Welcome{Error: "в игре уже есть второй игрок"}})
			c.Close()
			continue
		}
		h.client = k
		h.lastEv = 0
		h.status = ""
		h.msgs = append(h.msgs, "Второй игрок подключился: "+c.RemoteAddr().String())
		h.mu.Unlock()
		k.send(&Msg{Welcome: &Welcome{Side: 1 - h.side}})
		go h.readClient(k)
	}
}

func (h *Host) readClient(k *conn) {
	for {
		m, err := k.recv()
		if err != nil {
			h.mu.Lock()
			if h.client == k {
				h.client = nil
				h.status = "Второй игрок отключился. Ожидание переподключения…"
				h.w.Sides[1-h.side].Pausing = false
			}
			h.mu.Unlock()
			k.c.Close()
			return
		}
		if m.Cmd != nil {
			h.mu.Lock()
			c := *m.Cmd
			c.Side = 1 - h.side
			e := h.w.Apply(c)
			h.mu.Unlock()
			if e != "" {
				k.send(&Msg{Notice: e})
			}
		}
	}
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
			running := h.sandbox || h.client != nil
			if running {
				h.w.Update(dt)
				if h.ai != nil && (!h.w.Paused() || h.w.Placement) {
					h.ai.Tick(h.w)
				}
			}
			var v *sim.View
			k := h.client
			sendAcc += dt
			if k != nil && sendAcc >= 0.2 && !h.sending {
				sendAcc = 0
				v = h.w.BuildView(1-h.side, h.lastEv)
				h.sending = true
			}
			h.mu.Unlock()
			if v != nil {
				go func(k *conn, v *sim.View) {
					err := k.send(&Msg{View: v})
					h.mu.Lock()
					h.sending = false
					if err == nil {
						if n := len(v.Events); n > 0 && v.Events[n-1].ID > h.lastEv {
							h.lastEv = v.Events[n-1].ID
						}
					}
					h.mu.Unlock()
					if err != nil {
						k.c.Close()
					}
				}(k, v)
			}
		}
	}
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
	return h.status
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
	if h.ln != nil {
		h.ln.Close()
	}
	if h.client != nil {
		h.client.c.Close()
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
	side   int
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
	cl := &Client{k: k, side: m.Welcome.Side, status: "Получение данных…"}
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
	c.Side = cl.side
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

func (cl *Client) Side() int     { return cl.side }
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
