// Package sim — игровая симуляция. Работает только на стороне хоста;
// клиенты получают отфильтрованное туманом войны представление (View).
package sim

import (
	"math/rand"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/world"
)

// Pt — точка в км.
type Pt struct{ X, Y float64 }

// Состояния мобильного юнита.
const (
	UnitDeployed = iota
	UnitPacking
	UnitMoving
	UnitDeploying
)

// UnitStateNames — подписи состояний.
var UnitStateNames = []string{"развёрнут", "сворачивается", "на марше", "разворачивается"}

// Позиции фронта.
const (
	PostureDefense = iota
	PostureActive
	PostureOffense
)

// PostureNames — подписи позиций.
var PostureNames = []string{"Оборона", "Активная оборона", "Наступление"}

// DirNames — направления фронта.
var DirNames = [NumDir]string{"Киев", "Харьков", "Донбасс", "Крым"}

// NumDir — число направлений фронта.
const NumDir = 4

// SpeedMult — множители скоростей 1–5.
var SpeedMult = []float64{0, 1, 2, 3, 5, 8}

// Building — здание.
type Building struct {
	ID        uint32
	Type      string
	Name      string
	Side      int
	X, Y      float64
	HP        float64
	MaxHP     float64
	Built     float64 // 0..1, 1 — готово
	Masked    bool
	Dir       int // направление для логистики, -1 — авто
	Aircraft  map[string]float64
	Repair    bool
	Repairing bool // сейчас чинится бригадой
	Prewar    bool
	Budget    float64 // накопленные пуски
	Busy      int     // занятые каналы перехвата (аэродром)
	Placed    bool    // поставлен игроком на экране расстановки
	KeyHit    bool    // ключевой объект уже выведен из строя (повторно мораль не меняется до ремонта)
	Scale     float64 // множитель выпуска (0 — как 1)
	Region    int
}

// Unit — мобильный юнит.
type Unit struct {
	ID        uint32
	Type      string
	Side      int
	X, Y      float64
	HP        float64
	State     int
	Timer     float64 // минут до конца сворачивания/развёртывания
	Path      []Pt
	PathRail  []bool    // участок до Path[i] проезжается в эшелоне по ж/д
	PathWait  []float64 // минут ожидания (погрузка, выгрузка) перед участком до Path[i]
	Busy      int       // занятые каналы
	Reload    float64   // минут до готовности к залпу
	Ready     float64   // ПВО: ракет на пусковых
	FlashTill float64   // засветка после залпа
	Placed    bool      // поставлен игроком на экране расстановки
	Fire      int       // режим огня ПВО: 0 — по всем целям, 1 — без дронов и ложных целей, 2 — только баллистика
}

// Projectile — летящий боеприпас или БПЛА.
type Projectile struct {
	ID        uint32
	Munition  string
	Side      int
	X, Y      float64
	Path      []Pt // оставшиеся точки, последняя — цель
	Home      Pt
	Returning bool
	Delay     float64 // минут до старта
	Engaged   int
	Source    uint32
	Group     uint32
	Traveled  float64
	Heading   float64
}

// StrikeGroup — учёт одного удара для итогового донесения атакующему.
type StrikeGroup struct {
	Side     int
	Munition string
	Total    int
	Arrived  int
	Downed   int
	X, Y     float64
}

// Engagement — перехват в процессе.
type Engagement struct {
	AD   uint32 // юнит ПВО (0 — перехват самолётами)
	Bld  uint32 // аэродром, с которого взлетели перехватчики
	Proj uint32
	T    float64
	Pk   float64
	Side int
}

// Post — отложенное подтверждение прилёта в соцсетях (появится у атаковавшей стороны).
type Post struct {
	At  float64
	Bld uint32
}

// PlaceHint — стартовая позиция юнита из данных стороны.
type PlaceHint struct {
	Type string
	X, Y float64
}

// Contact — разведданные об объекте противника.
type Contact struct {
	ID     uint32
	Kind   int    // 0 здание, 1 юнит
	Type   string // как видится ("" — неизвестно)
	Class  string // словесный класс, если тип неизвестен
	X, Y   float64
	Seen   float64 // игровое время, мин (-1 — довоенные данные)
	HP     float64 // доля HP, -1 — неизвестно
	Source string
}

// Order — позиция госзаказа.
type Order struct {
	Item      string
	Remaining int // -1 — непрерывно
	Progress  float64
	Stalled   bool
}

// Delivery — ожидаемая поставка.
type Delivery struct {
	Name   string
	Item   string
	Amount float64
	At     float64
}

// Event — запись журнала.
type Event struct {
	ID     uint64
	Time   float64
	Text   string
	Level  int // 0 инфо, 1 важно, 2 тревога
	X, Y   float64
	HasPos bool
}

// Direction — силы направления.
type Direction struct {
	Men, Armor, Artillery, FPV float64
	FPVPow                     float64 // FPV с учётом версий: сумма (штук × сила версии)
	Supply                     float64
	Power                      float64
	Air                        float64
	Tiles                      int
	Losses                     float64 // потери людей, тыс., накопительно
	Gained, Lost               int     // тайлов за текущий час
	GainedH, LostH             int     // тайлов за прошлый час
}

// Capture — недавний захват тайла (для подсветки на карте).
type Capture struct {
	Tile int32
	Side int8
	Time float32
}

// Side — состояние стороны.
type Side struct {
	Res          data.Res
	Rates        data.Res
	Morale       float64
	People       float64
	LaborLoss    float64
	Stocks       map[string]float64
	Storage      data.FrontPool
	Dirs         [NumDir]Direction // силы по направлениям (Киев, Харьков, Донбасс, Крым)
	DirAlloc     [NumDir]float64   // доли пополнения по направлениям
	DirsSet      bool              // Dirs заполнены (иначе мигрируем из старых Front/Alloc/PostureDir на три направления)
	Front        [3]Direction      // устарело: три направления старых сохранений
	Alloc        [3]float64        // устарело
	Posture      int               // устарело: позиция всего фронта (старые сохранения)
	DirPosture   [NumDir]int       // позиция по направлениям
	PostureDir   [3]int            // устарело: позиции на трёх направлениях старых сохранений
	PostureSet   bool              // PostureDir заполнена (иначе мигрируем из Posture)
	HasMain      bool
	MainX        float64
	MainY        float64
	Orders       []Order
	Capacity     map[string]float64
	Unlocked     map[string]bool
	Researched   map[string]bool
	Research     string
	Progress     map[string]float64
	Bonus        map[string]float64 // очки трофеев и опыта по веткам
	ResRate      float64
	ResFund      int
	AgentFund    bool
	AgentTimer   float64
	Effects      map[string]float64
	Deliveries   []Delivery
	ImportCount  map[string]int
	AidDone      map[string]bool
	MobUsed      map[string]int
	MobReady     map[string]float64
	PropReady    float64              // время, с которого доступна следующая кампания
	MoraleHist   map[string][]float64 // время недавних успехов по видам (убывающая отдача)
	CitySeen     map[int]int          // сколько раз сторона брала город
	MissionSeen  map[string][]uint32  // задание → здания, уже засчитанные в прогресс
	MissionFail  map[string]bool      // задания, которые уже не выполнить
	StartAD      int                  // число комплексов тяжёлой ПВО на начало войны (для помощи «замена ПВО»)
	KeepStock    map[string]bool      // автозаказ ЗУР: держать запас (id боеприпаса)
	LowAlert     map[string]int       // предупреждения о запасе ЗУР: 1 — мало, 2 — кончились
	AutoImport   map[string]bool      // включена автозакупка предложения (по id)
	AutoImportAt map[string]float64   // когда автозакупка срабатывала в последний раз
	AirOpen      map[string]bool      // открытые для ударов стороны пакеты воздушного пространства
	SanctionOn   map[string]bool      // введённые против стороны пакеты санкций
	SanctionAt   map[string]float64   // когда введён пакет (игровые минуты)
	Posts        []Post               // отложенные подтверждения прилётов в соцсетях
	Reserve      map[string]int       // резерв для расстановки перед стартом: тип юнита или здания → штук
	Hints        []PlaceHint          // где эти юниты стояли по умолчанию (подсказки для ИИ)
	PalSub       bool                 // подписка на «ИИ Палантир»
	PalLast      PalRequest           // последний удар через «Палантир» (для «Повторить»)
	PalPreview   PalPreview           // последний рассчитанный план
	Ready        bool                 // расстановка завершена
	Known        map[uint32]*Contact
	SeenAt       []float32 // время последнего наблюдения тайла
	Events       []Event
	Speed        int
	Pausing      bool
	PauseLeft    float64
	LossAcc      float64
	Blackout     float64 // доля населения без света
	RegionPower  map[int]float64
	Power        [2]float64 // генерация, потребление
	Income       float64
}

// World — всё состояние партии.
type World struct {
	rec           *Recorder // журнал партии (не сохраняется)
	Time          float64   // игровые минуты с начала партии
	PrepEnd       float64
	Owner         []uint8 // 0 никто, 1 РФ, 2 Украина
	Fort          []uint8
	FortJobs      map[int]float64
	Pressure      []float32
	Sides         [2]*Side
	Buildings     map[uint32]*Building
	Units         map[uint32]*Unit
	Projs         map[uint32]*Projectile
	Engs          []Engagement
	Groups        map[uint32]*StrikeGroup
	Captures      []Capture
	SpawnN        [2]int // счётчик для чередования точек появления
	FrontHour     float64
	NextID        uint32
	NextEvent     uint64
	BomberWarnAt  [2]float64 // когда сторона получила последнее предупреждение о взлёте стратегической авиации
	FrontAcc      float64
	HourAcc       float64
	Winner        int
	WinReason     string
	Sandbox       bool
	Cheat         bool // песочница «всё открыто»: всё изучено, производство и стройка мгновенно и бесплатно
	Placement     bool // идёт расстановка перед стартом: время стоит
	PlacementDone bool // расстановка уже была (повторно не начинается)
	Solo          bool // одиночная игра против ИИ
	NetHost       bool // сетевая игра: скоростью и паузой управляет только хост
	HostSide      int  // сторона хоста
	Human         int  // сторона человека в одиночной игре (у старых сохранений 0 — Россия)
	Seed          int64

	cat         *data.Catalog
	m           *world.MapData
	rng         *rand.Rand
	depots      [5][]int // тайлы месторождений по типам
	kyiv        int      // тайл Киева
	cityAt      map[int]int
	frontT      [2][]int // фронтовые тайлы по сторонам (кэш)
	visible     [2][]bool
	reqCache    map[uint32]float64
	sens        sensorIndex
	dirMap      []uint8 // направление фронта каждого тайла
	dirPts      []Pt
	bridges     []bridgeLink // мосты с проездом техники (кэш)
	bridgesOK   bool
	vc          []victoryCity
	raids       map[[3]int]*raid // накапливаемые сводки налётов
	vcOK        bool
	nextVictory float64 // не проверять условия победы до этого времени (сбрасывается захватом тайла)
}

// Catalog и Map — доступ для интерфейса хоста.
func (w *World) Catalog() *data.Catalog { return w.cat }
func (w *World) Map() *world.MapData    { return w.m }

func (w *World) newID() uint32 {
	w.NextID++
	return w.NextID
}

// OwnerSide возвращает сторону-владельца тайла или -1.
func (w *World) OwnerSide(i int) int { return int(w.Owner[i]) - 1 }

// War — идёт ли война (подготовительная фаза закончилась).
func (w *World) War() bool { return w.Time >= w.PrepEnd }

// HoursSinceWar — игровые часы с начала войны.
func (w *World) HoursSinceWar() float64 {
	if !w.War() {
		return 0
	}
	return (w.Time - w.PrepEnd) / 60
}

// Log добавляет событие стороне.
func (w *World) Log(side int, level int, text string) {
	w.logAt(side, level, text, 0, 0, false)
}

// LogAt добавляет событие с координатами.
func (w *World) LogAt(side, level int, text string, x, y float64) {
	w.logAt(side, level, text, x, y, true)
}

func (w *World) logAt(side, level int, text string, x, y float64, has bool) {
	w.NextEvent++
	w.recEvent(side, level, text)
	s := w.Sides[side]
	s.Events = append(s.Events, Event{ID: w.NextEvent, Time: w.Time, Text: text, Level: level, X: x, Y: y, HasPos: has})
	if len(s.Events) > 300 {
		s.Events = s.Events[len(s.Events)-300:]
	}
}
