package netplay

import (
	"encoding/json"
	"net"
	"os"
	"sort"
	"sync"
	"time"
)

// Поиск игр в локальной сети и в Radmin VPN: хост каждые полторы секунды рассылает
// UDP-объявление на широковещательные адреса своих интерфейсов, а экран «Подключиться»
// слушает порт и показывает найденные лобби. Вводить IP не нужно (ручной ввод остался
// запасным путём, если широковещание в сети не проходит).

// DiscoveryPort — UDP-порт объявлений.
const DiscoveryPort = 27016

const announceMagic = "DroneEblaLobby1"

type announce struct {
	Magic   string
	Version int
	Hash    string
	Port    int
	Name    string
	Players int
	Started bool
}

// Found — найденная игра.
type Found struct {
	Name       string
	IP         string
	Port       int
	Players    int
	Started    bool
	Compatible bool // та же версия игры и те же данные
	seen       time.Time
}

// broadcastAddrs — адреса, на которые рассылается объявление.
func broadcastAddrs() []*net.UDPAddr {
	out := []*net.UDPAddr{{IP: net.IPv4bcast, Port: DiscoveryPort}}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.To4() == nil || ipn.IP.IsLoopback() {
			continue
		}
		ip, mask := ipn.IP.To4(), ipn.Mask
		if len(mask) != 4 {
			continue
		}
		b := make(net.IP, 4)
		for i := range b {
			b[i] = ip[i] | ^mask[i]
		}
		out = append(out, &net.UDPAddr{IP: b, Port: DiscoveryPort})
	}
	return out
}

// startAnnounce запускает рассылку объявлений, пока хост жив.
func (h *Host) startAnnounce() {
	name, _ := os.Hostname()
	if name == "" {
		name = "Игра"
	}
	port := h.ln.Addr().(*net.TCPAddr).Port
	go func() {
		c, err := net.ListenUDP("udp4", &net.UDPAddr{})
		if err != nil {
			return
		}
		defer c.Close()
		t := time.NewTicker(1500 * time.Millisecond)
		defer t.Stop()
		for {
			h.mu.Lock()
			a := announce{Magic: announceMagic, Version: Version, Hash: h.dataHash, Port: port, Name: name, Players: len(h.players) + 1, Started: h.started}
			h.mu.Unlock()
			b, _ := json.Marshal(a)
			for _, to := range broadcastAddrs() {
				c.WriteToUDP(b, to)
			}
			select {
			case <-h.stop:
				return
			case <-t.C:
			}
		}
	}()
}

// Discovery слушает объявления хостов.
type Discovery struct {
	mu    sync.Mutex
	c     *net.UDPConn
	hash  string
	found map[string]*Found
	err   error
}

// NewDiscovery начинает слушать порт поиска; hash — хеш данных игры для проверки совместимости.
func NewDiscovery(hash string) *Discovery {
	d := &Discovery{hash: hash, found: map[string]*Found{}}
	c, err := net.ListenUDP("udp4", &net.UDPAddr{Port: DiscoveryPort})
	if err != nil {
		d.err = err
		return d
	}
	d.c = c
	go d.loop()
	return d
}

func (d *Discovery) loop() {
	buf := make([]byte, 2048)
	for {
		n, from, err := d.c.ReadFromUDP(buf)
		if err != nil {
			return
		}
		var a announce
		if json.Unmarshal(buf[:n], &a) != nil || a.Magic != announceMagic {
			continue
		}
		ip := from.IP.String()
		key := net.JoinHostPort(ip, itoa(a.Port))
		d.mu.Lock()
		d.found[key] = &Found{Name: a.Name, IP: ip, Port: a.Port, Players: a.Players, Started: a.Started,
			Compatible: a.Version == Version && a.Hash == d.hash, seen: time.Now()}
		d.mu.Unlock()
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// List — актуальные игры (не старше 6 секунд), по имени.
func (d *Discovery) List() []Found {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []Found
	for k, f := range d.found {
		if time.Since(f.seen) > 6*time.Second {
			delete(d.found, k)
			continue
		}
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].IP < out[j].IP
	})
	return out
}

// Err — почему поиск не работает (порт занят и т. п.).
func (d *Discovery) Err() error { return d.err }

// Close останавливает поиск.
func (d *Discovery) Close() {
	if d != nil && d.c != nil {
		d.c.Close()
	}
}
