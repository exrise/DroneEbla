// Package data описывает игровые каталоги (здания, юниты, боеприпасы,
// технологии, стороны) и загружает их из JSON. Значения по умолчанию
// встроены в .exe; если рядом с .exe лежит папка data с файлами
// тех же имён, они перекрывают встроенные — так цифры можно править
// без пересборки.
package data

// Ресурсы.
const (
	ResMoney = iota
	ResFuel
	ResSteel
	ResElectronics
	ResAmmo
	NumRes
)

// ResKeys — ключи ресурсов в JSON.
var ResKeys = [NumRes]string{"money", "fuel", "steel", "electronics", "ammo"}

// ResNames — названия ресурсов в интерфейсе.
var ResNames = [NumRes]string{"Деньги", "Топливо", "Сталь", "Электроника", "Боеприпасы"}

// Res — набор ресурсов.
type Res [NumRes]float64

// Add прибавляет b*k.
func (r *Res) Add(b Res, k float64) {
	for i := range r {
		r[i] += b[i] * k
	}
}

// Covers — хватает ли ресурсов на b*k.
func (r Res) Covers(b Res, k float64) bool {
	for i := range r {
		if b[i]*k > r[i]+1e-9 {
			return false
		}
	}
	return true
}

// Стороны.
const (
	RU = 0
	UA = 1
)

// SideKeys — ключи сторон в JSON.
var SideKeys = [2]string{"ru", "ua"}

// SideNames — названия сторон.
var SideNames = [2]string{"Россия", "Украина"}

// Категории производственных мощностей (госзаказ).
const (
	CapAir    = "air"    // ракеты, КР, зенитные ракеты
	CapDrone  = "drone"  // ударные и разведывательные БПЛА, FPV
	CapGround = "ground" // бронетехника, артиллерия, мобильные комплексы
)

// BuildingType — тип здания.
type BuildingType struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Short        string             `json:"short"`
	HP           float64            `json:"hp"`
	Cost         map[string]float64 `json:"cost"`
	BuildHours   float64            `json:"build_hours"`
	Power        float64            `json:"power"` // + генерация, − потребление, МВт
	Produces     map[string]float64 `json:"produces"`
	Consumes     map[string]float64 `json:"consumes"`
	Capacity     map[string]float64 `json:"capacity"`     // очки мощности в час по категориям
	NeedDeposit  map[string]float64 `json:"need_deposit"` // тип → радиус, км (0 — где угодно на своей территории)
	NeedNear     string             `json:"need_near"`    // своё целое здание этого типа в радиусе
	NearKm       float64            `json:"near_km"`
	Research     float64            `json:"research"` // очки исследований в час
	Supply       float64            `json:"supply"`   // вклад в снабжение направления
	Transfer     float64            `json:"transfer"` // пропускная способность перетока энергии, МВт
	Launch       []string           `json:"launch"`   // платформы пуска: dronesite, strategic, naval, tactical
	LaunchRate   float64            `json:"launch_rate"`
	Aircraft     map[string]int     `json:"aircraft"` // tactical / strategic
	Untargetable bool               `json:"untargetable"`
	Buildable    []string           `json:"buildable"` // стороны, которым доступно строительство
	Vision       float64            `json:"vision"`
	Export       float64            `json:"export"`   // доход от экспорта (деньги в час) при целом здании
	OilPort      bool               `json:"oil_port"` // множитель нефтяного экспорта
	Bridge       bool               `json:"bridge"`
	Desc         string             `json:"desc"`
	RepairHours  float64            `json:"repair_hours"` // часов на полный ремонт одной бригадой
}

// RepairHrs — время полного ремонта, ч (если не задано — по прочности).
func (bt *BuildingType) RepairHrs() float64 {
	if bt.RepairHours > 0 {
		return bt.RepairHours
	}
	return 20 + 0.15*bt.HP
}

// UnitType — тип мобильного юнита.
type UnitType struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Short       string             `json:"short"`
	Side        string             `json:"side"`
	Kind        string             `json:"kind"` // ad, radar, reb, launcher, rtr
	HP          float64            `json:"hp"`
	SpeedRoad   float64            `json:"speed_road"`
	SpeedOff    float64            `json:"speed_off"`
	PackMin     float64            `json:"pack_min"`
	DeployMin   float64            `json:"deploy_min"`
	Vision      float64            `json:"vision"`
	RadarKm     float64            `json:"radar_km"`
	RadarLow    float64            `json:"radar_low"` // множитель дальности по низким целям
	RangeKm     float64            `json:"range_km"`
	Channels    int                `json:"channels"`
	Interceptor string             `json:"interceptor"` // id запаса зенитных ракет ("" — пушки, тратят боеприпасы)
	PkLow       float64            `json:"pk_low"`
	PkHigh      float64            `json:"pk_high"`
	EngageSec   float64            `json:"engage_sec"`
	Magazine    int                `json:"magazine"` // ПВО: готовых к пуску ракет (очередей у пушек); перезарядка — reload_min
	RebKm       float64            `json:"reb_km"`
	RebPower    float64            `json:"reb_power"`
	RtrKm       float64            `json:"rtr_km"`
	Munitions   []string           `json:"munitions"`
	Salvo       int                `json:"salvo"`
	ReloadMin   float64            `json:"reload_min"`
	Cost        map[string]float64 `json:"cost"`
	Cap         string             `json:"cap"`
	CapPoints   float64            `json:"cap_points"`
	Desc        string             `json:"desc"`
}

// Emitter — излучает ли юнит (виден РТР).
func (u *UnitType) Emitter() bool { return u.RadarKm > 0 || u.RebKm > 0 }

// MunitionType — боеприпас, БПЛА или зенитная ракета (запас).
type MunitionType struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Short      string             `json:"short"`
	Side       string             `json:"side"`
	Kind       string             `json:"kind"`     // ballistic, cruise, drone, decoy, recon, rocket, interceptor
	Class      string             `json:"class"`    // low / high
	Platform   string             `json:"platform"` // launcher, dronesite, strategic, naval
	SpeedKmh   float64            `json:"speed_kmh"`
	RangeKm    float64            `json:"range_km"`
	Damage     float64            `json:"damage"`
	BlastKm    float64            `json:"blast_km"`
	Accuracy   float64            `json:"accuracy"`
	GPS        bool               `json:"gps"`
	Stealth    float64            `json:"stealth"` // 0..1, уменьшает дальность обнаружения
	Vision     float64            `json:"vision"`  // для разведывательных БПЛА
	EnduranceH float64            `json:"endurance_h"`
	Cost       map[string]float64 `json:"cost"`
	Cap        string             `json:"cap"`
	CapPoints  float64            `json:"cap_points"`
	Desc       string             `json:"desc"`
}

// FrontType — снаряжение фронта (бронетехника, артиллерия, FPV).
type FrontType struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Cost      map[string]float64 `json:"cost"`
	Cap       string             `json:"cap"`
	CapPoints float64            `json:"cap_points"`
	Batch     float64            `json:"batch"` // единиц за один заказ
}

// Tech — узел дерева технологий.
type Tech struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Branch   string             `json:"branch"` // drones, strike, ad, industry
	Cost     float64            `json:"cost"`
	Requires []string           `json:"requires"`
	Unlocks  []string           `json:"unlocks"`
	Effects  map[string]float64 `json:"effects"`
	Desc     string             `json:"desc"`
}

// ImportOffer — закупка за рубежом.
type ImportOffer struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Item     string  `json:"item"`   // id боеприпаса/юнита/снаряжения или ресурс res:<ключ>
	Amount   float64 `json:"amount"` // количество за одну покупку
	Money    float64 `json:"money"`
	DelayH   float64 `json:"delay_h"`
	Limit    int     `json:"limit"`    // максимум покупок (0 — без лимита)
	Requires string  `json:"requires"` // технология, после которой предложение доступно
	Removes  string  `json:"removes"`  // технология, после которой предложение исчезает
}

// AidPackage — пакет помощи.
type AidPackage struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	AtHour    float64            `json:"at_hour"`    // через сколько игровых часов после начала войны
	MinMorale float64            `json:"min_morale"` // условие: мораль не ниже
	NeedKyiv  bool               `json:"need_kyiv"`  // условие: Киев удержан
	Items     map[string]float64 `json:"items"`      // id → количество (res:<ключ> для ресурсов)
	Morale    float64            `json:"morale"`
}

// Mobilization — вариант пополнения людьми.
type Mobilization struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Men       float64 `json:"men"`
	Morale    float64 `json:"morale"` // изменение морали
	Labor     float64 `json:"labor"`  // доля потерянной рабочей силы (0.05 = −5% выпуска)
	Money     float64 `json:"money"`
	CooldownH float64 `json:"cooldown_h"`
	Limit     int     `json:"limit"`
}

// Satellite — спутник стороны.
type Satellite struct {
	Name    string    `json:"name"`
	Sensor  string    `json:"sensor"` // optical / radar
	PeriodH float64   `json:"period_h"`
	PhaseH  float64   `json:"phase_h"`
	SwathKm float64   `json:"swath_km"`
	Tracks  []float64 `json:"tracks"` // смещения трасс по долготе (градусы), по кругу
	Angle   float64   `json:"angle"`  // наклон трассы, градусы от меридиана
}

// StartUnit — юнит на старте.
type StartUnit struct {
	Type string  `json:"type"`
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
}

// FrontPool — силы направления.
type FrontPool struct {
	Men       float64 `json:"men"`
	Armor     float64 `json:"armor"`
	Artillery float64 `json:"artillery"`
	FPV       float64 `json:"fpv"`
}

// SideDef — стартовые условия и особенности стороны.
type SideDef struct {
	Name          string             `json:"name"`
	Resources     map[string]float64 `json:"resources"`
	Morale        float64            `json:"morale"`
	People        float64            `json:"people"`       // мобилизационный резерв
	TaxPerCity    float64            `json:"tax_per_city"` // деньги в час за 100 тыс. жителей
	BaseIncome    float64            `json:"base_income"`
	MenStream     float64            `json:"men_stream"` // приток людей на фронт, тыс./ч (из резерва)
	EntryLon      float64            `json:"entry_lon"`  // точка появления поставок
	EntryLat      float64            `json:"entry_lat"`
	Entries       []EntryPoint       `json:"entries"` // пункты въезда импорта и помощи (по кругу); если пусто — entry_lon/lat
	Stocks        map[string]float64 `json:"stocks"`  // запасы боеприпасов и зенитных ракет
	Storage       FrontPool          `json:"storage"` // техника на хранении (советские склады)
	Front         [3]FrontPool       `json:"front"`   // Север, Донбасс, Юг
	Units         []StartUnit        `json:"units"`
	Unlocked      []string           `json:"unlocked"` // доступно для производства с начала
	Imports       []ImportOffer      `json:"imports"`
	Aid           []AidPackage       `json:"aid"`
	Mobilization  []Mobilization     `json:"mobilization"`
	Satellites    []Satellite        `json:"satellites"`
	BomberWarning bool               `json:"bomber_warning"` // предупреждение о взлёте стратегов противника
	BelarusAir    bool               `json:"belarus_air"`
}

// EntryPoint — пункт въезда поставок (граница, порт).
type EntryPoint struct {
	Name string  `json:"name"`
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
}

// Object — реальный объект на карте на старте.
type Object struct {
	Type string  `json:"type"`
	Name string  `json:"name"`
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
	Dir  int     `json:"dir"` // для мостов и узлов: направление (-1 — авто)
}

// Rules — общие параметры.
type Rules struct {
	GameMinPerSec       float64            `json:"game_min_per_sec"`
	PrepMinutes         float64            `json:"prep_minutes"` // подготовительная фаза, игровые минуты
	PauseBudgetSec      float64            `json:"pause_budget_sec"`
	CityTaxRadiusKm     float64            `json:"city_tax_radius_km"`
	FrontStepMin        float64            `json:"front_step_min"`
	FrontAttack         float64            `json:"front_attack"`
	FrontCapture        float64            `json:"front_capture"`
	FrontThreshold      float64            `json:"front_threshold"` // минимальное превосходство для продвижения
	FrontLoss           float64            `json:"front_loss"`
	FrontAmmoUse        float64            `json:"front_ammo_use"`
	FrontFuelUse        float64            `json:"front_fuel_use"`
	MainEffortKm        float64            `json:"main_effort_km"`
	MainEffortMult      float64            `json:"main_effort_mult"`
	FortPerLevel        float64            `json:"fort_per_level"`
	FortCost            map[string]float64 `json:"fort_cost"`
	FortHours           float64            `json:"fort_hours"`
	RiverDefense        float64            `json:"river_defense"`
	UrbanDefense        float64            `json:"urban_defense"`
	AirBonusKm          float64            `json:"air_bonus_km"`
	AirBonusPerPlane    float64            `json:"air_bonus_per_plane"`
	AirLossPerAD        float64            `json:"air_loss_per_ad"`
	FrontVisionKm       float64            `json:"front_vision_km"`
	BuildingVisionKm    float64            `json:"building_vision_km"`
	AirWatchKm          float64            `json:"air_watch_km"` // визуальное наблюдение за воздухом вокруг своих объектов
	AgentEveryH         float64            `json:"agent_every_h"`
	AgentErrorKm        float64            `json:"agent_error_km"`
	AgentFundCost       float64            `json:"agent_fund_cost"`
	FlashMin            float64            `json:"flash_min"`       // засветка пусковой после залпа
	RepairPerHour       float64            `json:"repair_per_hour"` // доля HP в час
	RepairCostK         float64            `json:"repair_cost_k"`
	RepairCrews         int                `json:"repair_crews"`           // зданий, которые сторона чинит одновременно
	StrikeDroneVisionKm float64            `json:"strike_drone_vision_km"` // обзор ударного дрона после исследования разведки
	RepairPerType       int                `json:"repair_per_type"`        // зданий одного типа, которые чинятся одновременно
	RepairRuinMult      float64            `json:"repair_ruin_mult"`       // скорость ремонта почти разрушенного здания
	DamageFloor         float64            `json:"damage_floor"`           // ниже этой доли HP здание не производит
	DamageCurve         float64            `json:"damage_curve"`           // выпуск = ((hp−порог)/(1−порог))^кривая
	MaskCost            map[string]float64 `json:"mask_cost"`
	ResearchBase        float64            `json:"research_base"`
	ResearchFundCost    float64            `json:"research_fund_cost"`
	TrophyPoints        float64            `json:"trophy_points"`
	ExperiencePoints    float64            `json:"experience_points"`
	MoraleBlackout      float64            `json:"morale_blackout"`
	MoraleLossPer10k    float64            `json:"morale_loss_per_10k"`
	MoraleCity          float64            `json:"morale_city"`
	MoraleDebt          float64            `json:"morale_debt"`
	MoraleRecover       float64            `json:"morale_recover"`
	MenPerPeople        float64            `json:"men_per_people"`
	CaptureDamage       float64            `json:"capture_damage"`
	PowerPerCity        float64            `json:"power_per_city"` // потребление МВт на 100 тыс. жителей
	BuildRadiusKm       float64            `json:"build_radius_km"`
}

// Catalog — все данные игры.
type Catalog struct {
	Rules     Rules
	Buildings []BuildingType
	Units     []UnitType
	Munitions []MunitionType
	Front     []FrontType
	Tech      map[string][]Tech // по сторонам
	Sides     [2]SideDef
	Objects   []Object

	BuildingByID map[string]*BuildingType
	UnitByID     map[string]*UnitType
	MunitionByID map[string]*MunitionType
	FrontByID    map[string]*FrontType
	TechByID     [2]map[string]*Tech
}

// ToRes переводит map в Res.
func ToRes(m map[string]float64) Res {
	var r Res
	for i, k := range ResKeys {
		r[i] = m[k]
	}
	return r
}

// SideIndex возвращает индекс стороны по ключу.
func SideIndex(s string) int {
	if s == "ua" {
		return UA
	}
	return RU
}
