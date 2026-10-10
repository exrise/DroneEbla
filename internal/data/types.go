// Package data описывает игровые каталоги (здания, юниты, боеприпасы,
// технологии, стороны) и загружает их из JSON. Значения по умолчанию
// встроены в .exe; если рядом с .exe лежит папка data с файлами
// тех же имён, они перекрывают встроенные — так цифры можно править
// без пересборки.
package data

import "fmt"

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

// Деньги в игре — миллионы национальной валюты: рубли у России (сторона 0), гривны у Украины (сторона 1).
var (
	MoneyUnit  = [2]string{"млн руб.", "млн грн"}   // единица в тексте: «120 млн руб.»
	MoneyLabel = [2]string{"Рубли, млн", "Гривны, млн"} // подпись ячейки и ресурса
)

// ResName — название ресурса i в интерфейсе стороны side (деньги — в её валюте).
func ResName(i, side int) string {
	if i == ResMoney && side >= 0 && side < 2 {
		return MoneyLabel[side]
	}
	return ResNames[i]
}

// MoneyText — сумма в валюте стороны: «120 млн руб.».
func MoneyText(side int, v float64) string {
	if side < 0 || side > 1 {
		side = 0
	}
	return fmt.Sprintf("%.0f %s", v, MoneyUnit[side])
}

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
	NeedCityKm   float64            `json:"need_city_km"`  // строить не дальше этого расстояния от своего города (0 — где угодно)
	TaxH         float64            `json:"tax_h"`         // налоги: денег в час при целом здании (с множителем scale)
	NeedWaterKm  float64            `json:"need_water_km"` // строить не дальше этого расстояния от реки, озера или моря (0 — где угодно)
	Research     float64            `json:"research"`      // очки исследований в час
	Supply       float64            `json:"supply"`        // вклад в снабжение направления
	Transfer     float64            `json:"transfer"`      // пропускная способность перетока энергии, МВт
	Launch       []string           `json:"launch"`        // платформы пуска: dronesite, strategic, naval, tactical
	LaunchRate   float64            `json:"launch_rate"`
	Aircraft     map[string]int     `json:"aircraft"` // tactical / strategic / fighter
	Intercept    *InterceptDef      `json:"intercept"`
	Untargetable bool               `json:"untargetable"`
	Buildable    []string           `json:"buildable"` // стороны, которым доступно строительство
	Vision       float64            `json:"vision"`
	Export       float64            `json:"export"`   // доход от экспорта (деньги в час) при целом здании
	OilPort      bool               `json:"oil_port"` // множитель нефтяного экспорта
	Bridge       bool               `json:"bridge"`
	Desc         string             `json:"desc"`
	Key          bool               `json:"key"`          // ключевой объект: его поражение влияет на мораль
	RepairHours  float64            `json:"repair_hours"` // часов на полный ремонт одной бригадой
}

// RepairHrs — время полного ремонта, ч (если не задано — по прочности).
func (bt *BuildingType) RepairHrs() float64 {
	if bt.RepairHours > 0 {
		return bt.RepairHours
	}
	return 20 + 0.15*bt.HP
}

// InterceptDef — перехват самолётами с аэродрома.
type InterceptDef struct {
	Km        float64 `json:"km"`         // радиус от аэродрома
	Channels  int     `json:"channels"`   // максимум одновременных перехватов
	PkLow     float64 `json:"pk_low"`     // вероятность сбить низкую цель
	EngageSec float64 `json:"engage_sec"` // время одного перехвата
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
	SpawnAt     string             `json:"spawn_at"` // тип здания, у которого появляется юнит (по умолчанию — завод своей категории)
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
	Evasion    float64            `json:"evasion"` // доля, на которую снижается вероятность перехвата (манёвр, гиперзвук)
	GPS        bool               `json:"gps"`
	Stealth    float64            `json:"stealth"` // 0..1, уменьшает дальность обнаружения
	Vision     float64            `json:"vision"`  // для разведывательных БПЛА
	EnduranceH float64            `json:"endurance_h"`
	Cost       map[string]float64 `json:"cost"`
	Cap        string             `json:"cap"`
	CapPoints  float64            `json:"cap_points"`
	Batch      float64            `json:"batch"` // штук за один цикл производства (0 — по одной): цена и очки мощности умножаются
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
	Power     float64            `json:"power"` // вес единицы на фронте (для FPV: версия дрона); 0 — обычный
}

// Tech — узел дерева технологий.
type Tech struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Branch string  `json:"branch"` // drones, strike, ad, intel — откуда берутся очки трофеев и опыта
	Cost   float64 `json:"cost"`
	// Line и Step — линейка версий (см. TechLine) и номер ступени в ней (с 1).
	// Предыдущая ступень той же линейки требуется автоматически.
	Line     string             `json:"line"`
	Step     int                `json:"step"`
	Requires []string           `json:"requires"`
	Unlocks  []string           `json:"unlocks"`
	Effects  map[string]float64 `json:"effects"`
	Desc     string             `json:"desc"`
}

// TechLine — линейка версий одного образца: «Герань-2 → 3 → 4 → 5».
// Start — версии, доступные с начала игры (показываются первыми карточками);
// дальше идут исследования, у которых Line совпадает с ID линейки, по Step.
type TechLine struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Group string   `json:"group"` // strike, ad, intel, front, caps
	Start []string `json:"start"`
}

// ImportOffer — закупка за рубежом.
type ImportOffer struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Item      string  `json:"item"`   // id боеприпаса/юнита/снаряжения или ресурс res:<ключ>
	Amount    float64 `json:"amount"` // количество за одну покупку
	Money     float64 `json:"money"`
	DelayH    float64 `json:"delay_h"`
	Limit     int     `json:"limit"`      // максимум покупок (0 — без лимита)
	AutoBelow float64 `json:"auto_below"` // для ресурсов: порог, ниже которого работает автозакупка (0 — автозакупки нет)
	Requires  string  `json:"requires"`   // технология, после которой предложение доступно
	Removes   string  `json:"removes"`    // технология, после которой предложение исчезает
}

// AidPackage — пакет помощи.
type AidPackage struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	AtHour    float64            `json:"at_hour"`     // через сколько игровых часов после начала войны
	MinMorale float64            `json:"min_morale"`  // условие: мораль не ниже
	NeedKyiv  bool               `json:"need_kyiv"`   // условие: Киев удержан
	MinADLoss float64            `json:"min_ad_loss"` // условие: потеряна не менее этой доли стартового парка тяжёлой ПВО (0 — без условия)
	Items     map[string]float64 `json:"items"`       // id → количество (res:<ключ> для ресурсов)
	Morale    float64            `json:"morale"`
	Title     string             `json:"title"`   // задание: заголовок (если пакет выдаётся за задание)
	Hint      string             `json:"hint"`    // задание: пояснение
	Mission   *MissionDef        `json:"mission"` // если задано — пакет выдаётся только за выполнение
}

// MissionDef — задание с наградой (пакет помощи).
type MissionDef struct {
	Kind     string  `json:"kind"`    // disable_building, disable_type, hit_region, hold, capture
	Target   string  `json:"target"`  // имя здания / тип здания / название города (hold, capture)
	Count    int     `json:"count"`   // сколько объектов (по умолчанию 1)
	HpFrac   float64 `json:"hp_frac"` // объект «поражён», когда его HP опускается ниже этой доли (по умолчанию 0.1)
	Lon      float64 `json:"lon"`     // центр области (hit_region, disable_type)
	Lat      float64 `json:"lat"`
	RadiusKm float64 `json:"radius_km"` // радиус области (0 — вся карта)
	Hours    float64 `json:"hours"`     // hold: сколько часов войны держать
}

// SanctionPackage — пакет санкций против стороны: вводится по таймеру (AtHour,
// с условиями) или в ответ на её действия (Trigger) и навсегда ухудшает экономику.
type SanctionPackage struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Hint        string             `json:"hint"`
	AtHour      float64            `json:"at_hour"`      // через сколько часов войны (если нет Trigger)
	NeedKyiv    bool               `json:"need_kyiv"`    // только если Киев держится (Запад уверен в Украине)
	EnemyMorale float64            `json:"enemy_morale"` // только если мораль противника не ниже
	Trigger     *MissionDef        `json:"trigger"`      // ответ на действия: disable_building, disable_type, hit_region, capture
	Effects     map[string]float64 `json:"effects"`      // штрафы, доли: tax, export, electronics, import_cost
	Morale      float64            `json:"morale"`       // разовое изменение морали стороны
}

// AirspacePackage — открытие воздушного пространства стран для ударов стороны:
// по таймеру (AtHour) и условиям. Открытое небо не закрывается.
type AirspacePackage struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Countries []string `json:"countries"` // коды стран (world.CountryCodes): pl, lt, lv, ee, fi
	AtHour    float64  `json:"at_hour"`   // через сколько часов войны
	NeedKyiv  bool     `json:"need_kyiv"` // только если Киев держится
	MinMorale float64  `json:"min_morale"`
}

// SanctionKeys — виды штрафов санкций.
var SanctionKeys = map[string]string{
	"tax":         "налоги и базовый доход",
	"export":      "доход от экспорта нефти и зерна",
	"electronics": "выпуск электроники",
	"import_cost": "цены импорта",
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
	Foreign   bool    `json:"foreign"` // наёмники: не расходуют мобилизационный резерв и не снижают выпуск
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
	Tech    string    `json:"tech"`   // исследование, после которого спутник работает ("" — с начала)
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
	Name             string             `json:"name"`
	Resources        map[string]float64 `json:"resources"`
	Morale           float64            `json:"morale"`
	People           float64            `json:"people"`       // мобилизационный резерв
	TaxPerCity       float64            `json:"tax_per_city"` // деньги в час за 100 тыс. жителей
	BaseIncome       float64            `json:"base_income"`
	MenStream        float64            `json:"men_stream"` // приток людей на фронт, тыс./ч (из резерва)
	EntryLon         float64            `json:"entry_lon"`  // точка появления поставок
	EntryLat         float64            `json:"entry_lat"`
	Entries          []EntryPoint       `json:"entries"` // пункты въезда импорта и помощи (по кругу); если пусто — entry_lon/lat
	Stocks           map[string]float64 `json:"stocks"`  // запасы боеприпасов и зенитных ракет
	Storage          FrontPool          `json:"storage"` // техника на хранении (советские склады)
	Front            []FrontPool        `json:"front"`   // Киев, Харьков, Донбасс, Крым
	Units            []StartUnit        `json:"units"`
	ReserveBuildings map[string]int     `json:"reserve_buildings"` // здания (площадки), выдаваемые в резерв для расстановки
	Unlocked         []string           `json:"unlocked"`          // доступно для производства с начала
	Imports          []ImportOffer      `json:"imports"`
	Aid              []AidPackage       `json:"aid"`
	Sanctions        []SanctionPackage  `json:"sanctions"` // санкции против этой стороны
	Airspace         []AirspacePackage  `json:"airspace"`  // открытие неба соседних стран для ударов этой стороны
	Mobilization     []Mobilization     `json:"mobilization"`
	Satellites       []Satellite        `json:"satellites"`
	BomberWarning    bool               `json:"bomber_warning"` // предупреждение о взлёте стратегов противника
	BelarusAir       bool               `json:"belarus_air"`
	Palantir         *PalantirDef       `json:"palantir"` // ИИ-планировщик ударов по подписке (nil — у стороны его нет)
}

// PalantirDef — «ИИ Палантир»: подписка и требования к ЦОД.
type PalantirDef struct {
	MoneyH   float64 `json:"money_h"`   // плата за подписку, денег в час
	Building string  `json:"building"`  // здание-ЦОД
	MinPower float64 `json:"min_power"` // минимальная обеспеченность энергией области ЦОДа (0..1)
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
	// Link — для моста с проездом техники: два конца [lon, lat] на разных берегах; пока мост цел, юниты проходят между ними.
	Link [][2]float64 `json:"link"`
	// Scale — множитель выпуска (0 — как 1): мощность комбината, включая то, что за краем карты.
	Scale float64 `json:"scale"`
	// Aircraft — своё число самолётов у объекта (перекрывает значения типа по ключам).
	Aircraft map[string]int `json:"aircraft"`
}

// DirAnchor — опорная точка направления фронта: тайл относится к направлению с ближайшей точкой.
type DirAnchor struct {
	Name string  `json:"name"`
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
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
	AgentFundCost       float64            `json:"agent_fund_cost"`
	FlashMin            float64            `json:"flash_min"`       // засветка пусковой после залпа
	RepairPerHour       float64            `json:"repair_per_hour"` // доля HP в час
	RepairCostK         float64            `json:"repair_cost_k"`
	RepairCrews         int                `json:"repair_crews"`       // зданий, которые сторона чинит одновременно
	MoraleGood          float64            `json:"morale_good"`        // выше этой морали — бонусы
	MoraleBad           float64            `json:"morale_bad"`         // ниже этой морали — штрафы
	MoraleFrontHigh     float64            `json:"morale_front_high"`  // множитель силы фронта при морали 100
	MoraleFrontLow      float64            `json:"morale_front_low"`   // множитель силы фронта при морали 0
	MoraleLossMax       float64            `json:"morale_loss_max"`    // множитель потерь фронта при морали 0
	MoraleProdHigh      float64            `json:"morale_prod_high"`   // множитель выпуска и налогов при морали 100
	MoraleProdLow       float64            `json:"morale_prod_low"`    // множитель выпуска и налогов при морали 0
	MoraleLevyHigh      float64            `json:"morale_levy_high"`   // множитель притока людей при морали 100
	MoraleLevyLow       float64            `json:"morale_levy_low"`    // множитель притока людей и мобилизации при морали 0
	MoraleMobPenalty    float64            `json:"morale_mob_penalty"` // во сколько раз растёт штраф морали от мобилизации при морали 0
	PropagandaCost      float64            `json:"propaganda_cost"`    // деньги (×(1+мораль/100))
	PropagandaGain      float64            `json:"propaganda_gain"`    // прирост морали при морали 0 (убывает к 100)
	PropagandaCooldownH float64            `json:"propaganda_cooldown_h"`
	MoraleKeyHit        float64            `json:"morale_key_hit"`         // бонус атакующему за вывод из строя ключевого объекта
	MoraleKeyLoss       float64            `json:"morale_key_loss"`        // потеря морали владельцем ключевого объекта
	MoraleRepel         float64            `json:"morale_repel"`           // бонус обороне, если массированный удар полностью отбит
	MoraleRepelMin      int                `json:"morale_repel_min"`       // минимальный размер такого удара
	StrikeDroneVisionKm float64            `json:"strike_drone_vision_km"` // обзор ударного дрона после исследования разведки
	PlacementKinds      []string           `json:"placement_kinds"`        // виды юнитов, которые игрок сам расставляет перед стартом
	LowInterceptor      float64            `json:"low_interceptor"`        // запас ЗУР, при котором приходит предупреждение (не меньше 4 магазинов самого заряженного комплекса)
	KeepStockBatch      int                `json:"keep_stock_batch"`       // размер партии автозаказа ЗУР
	SellKeepMin         float64            `json:"sell_keep_min"`          // продажа излишков не трогает этот остаток ресурса
	SellRate            map[string]float64 `json:"sell_rate"`              // курс продажи излишков: ключ ресурса → денег за единицу
	Directions          []DirAnchor        `json:"directions"`             // опорные точки четырёх направлений фронта (Киев, Харьков, Донбасс, Крым)
	BomberWarnMin       float64            `json:"bomber_warn_min"`        // предупреждение о взлёте стратегической авиации — не чаще раза за столько игровых минут
	RuVictoryCities     []string           `json:"ru_victory_cities"`      // города, которые Россия должна удерживать одновременно для победы
	RetreatTiles        int                `json:"retreat_tiles"`          // радиус отхода юнита с захваченного тайла
	RetreatDamage       float64            `json:"retreat_damage"`         // доля прочности, теряемая при отступлении
	CityMinPop          int                `json:"city_min_pop"`           // город, возле которого строят заводы (население не меньше)
	BridgePassFrac      float64            `json:"bridge_pass_frac"`       // мост пропускает технику, пока его HP не ниже этой доли
	AutoImportReserve   float64            `json:"auto_import_reserve"`    // автозакупка не трогает деньги ниже этого запаса
	AutoImportEveryMin  float64            `json:"auto_import_every_min"`  // не чаще раза за столько игровых минут на предложение
	RailKmh             float64            `json:"rail_kmh"`               // скорость юнита в эшелоне по ж/д, км/ч
	RailBoardMin        float64            `json:"rail_board_min"`         // погрузка на станции, мин
	RailAlightMin       float64            `json:"rail_alight_min"`        // выгрузка, мин
	RailStationTiles    int                `json:"rail_station_tiles"`     // радиус вокруг работающей станции или узла, где можно сесть и выйти, тайлов
	PlacementBuildings  []string           `json:"placement_buildings"`    // типы стартовых зданий, которые игрок тоже расставляет сам (центры подготовки)
	SocialMinPop        int                `json:"social_min_pop"`         // соцсети: минимальное население рядом
	SocialCityKm        float64            `json:"social_city_km"`         // соцсети: радиус от города
	SocialDelayMin      float64            `json:"social_delay_min"`       // соцсети: задержка подтверждения, мин
	SocialDelayMax      float64            `json:"social_delay_max"`
	IntelPoints         float64            `json:"intel_points"`       // очки ветки «Разведка» за новую метку
	ForcesPoints        float64            `json:"forces_points"`      // очки ветки «Войска» за захват тайла
	AircraftLoss        float64            `json:"aircraft_loss"`      // вероятность потерять истребитель за перехват
	MoraleGainRef       float64            `json:"morale_gain_ref"`    // при этой морали успехи дают полный прирост; выше — меньше
	MoraleGainFloor     float64            `json:"morale_gain_floor"`  // минимальная доля прироста
	MoraleFatigue       float64            `json:"morale_fatigue"`     // спад прироста за каждый недавний успех того же вида
	MoraleFatigueH      float64            `json:"morale_fatigue_h"`   // за сколько игровых часов считаются недавние успехи
	MoraleCityRepeat    float64            `json:"morale_city_repeat"` // доля награды за повторный захват города
	RepairPerType       int                `json:"repair_per_type"`    // зданий одного типа, которые чинятся одновременно
	RepairRuinMult      float64            `json:"repair_ruin_mult"`   // скорость ремонта почти разрушенного здания
	DamageFloor         float64            `json:"damage_floor"`       // ниже этой доли HP здание не производит
	DamageCurve         float64            `json:"damage_curve"`       // выпуск = ((hp−порог)/(1−порог))^кривая
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
	Lines     map[string][]TechLine
	Sides     [2]SideDef
	Objects   []Object
	AI        AIConfig

	BuildingByID map[string]*BuildingType
	UnitByID     map[string]*UnitType
	MunitionByID map[string]*MunitionType
	FrontByID    map[string]*FrontType
	TechByID     [2]map[string]*Tech
	LineByID     [2]map[string]*TechLine
	LineSteps    [2]map[string][]*Tech // ступени линейки по порядку
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

// AIConfig — параметры компьютерного противника (ai.json).
type AIConfig struct {
	ThinkMin float64           `json:"think_min"` // как часто ИИ принимает решения, игровые минуты
	Sides    map[string]AISide `json:"sides"`     // по ключам сторон (ru, ua)
}

// AIBuild — что ИИ достраивает.
type AIBuild struct {
	Type     string  `json:"type"`
	Max      int     `json:"max"`       // сколько зданий такого типа держать
	MinMoney float64 `json:"min_money"` // строить, только если денег не меньше
	// Healthy — считать только целые (HP не ниже половины) здания: разрушенные заменяются новыми.
	Healthy bool `json:"healthy"`
	// When — условие: "" всегда, "power_low" — энергии не хватает или есть блэкаут.
	When string `json:"when"`
}

// AISide — поведение ИИ за одну сторону.
type AISide struct {
	// Smart включает «умное» поведение (экономика с парком ПВО и стройкой, позиции по силам, массированные залпы с приманками);
	// false — прежний простой бот (для сравнения и лёгкого уровня).
	Smart bool `json:"smart"`
	// Экономика.
	Research        []string  `json:"research"`         // приоритет исследований; остальные — по стоимости
	FundLevels      []float64 `json:"fund_levels"`      // порог денег для финансирования науки 1, 2, 3
	Orders          []string  `json:"orders"`           // позиции госзаказа по приоритету
	Imports         []string  `json:"imports"`          // закупки (id) по приоритету
	ImportReserve   float64   `json:"import_reserve"`   // денег не тратить на импорт ниже этого запаса
	ImportBelow     float64   `json:"import_below"`     // ресурс докупать, если его меньше (для предметов — штук)
	MobilizeBelow   float64   `json:"mobilize_below"`   // мобилизация, если людей на фронте меньше, тыс.
	PropagandaBelow float64   `json:"propaganda_below"` // кампания, если мораль ниже
	Build           []AIBuild `json:"build"`
	BuildEveryMin   float64   `json:"build_every_min"`
	BuildPerCycle   int       `json:"build_per_cycle"` // сколько зданий строить за один заход (по умолчанию 1)
	// ImportReserveCritical — резерв денег, когда ресурса почти нет; RichMoney/RichElecBelow — при избытке денег электронику докупают до этого запаса без резерва.
	ImportReserveCritical float64 `json:"import_reserve_critical"`
	RichMoney             float64 `json:"rich_money"`
	RichElecBelow         float64 `json:"rich_elec_below"`
	// StockCap — колпаки запаса дешёвых позиций (ложные цели, дроны-перехватчики): при превышении заказ снимается.
	StockCap map[string]float64 `json:"stock_cap"`
	// HeavyMenFrac — тяжёлая мобилизация, когда люди на фронте упали ниже этой доли максимума (по умолчанию 0,6).
	HeavyMenFrac   float64 `json:"heavy_men_frac"`
	ImportParallel int     `json:"import_parallel"` // сколько одинаковых закупок держать в пути одновременно
	// Fleet — целевой парк юнитов: "новый|старый:N" — держать N штук (живых и заказанных); недостающее заказывается.
	Fleet         []string `json:"fleet"`
	FleetBatch    int      `json:"fleet_batch"`    // не больше стольких штук одной позиции в одном заказе
	ContractMoney float64  `json:"contract_money"` // платную мобилизацию без потерь морали использовать, если денег больше
	PeakMenFrac   float64  `json:"peak_men_frac"`  // мобилизовать, когда людей на фронте меньше этой доли от максимума
	// SellAbove — умный режим продаёт излишки ресурса (ключ: fuel, steel), когда запас выше порога.
	SellAbove map[string]float64 `json:"sell_above"`
	// Corridors — цепочки путевых точек [lon, lat] над открытым небом соседних стран (запад → север):
	// ИИ летит по ним, когда прямой маршрут закрыт или не долетает.
	Corridors [][][2]float64 `json:"corridors"`
	// Фронт.
	PrepPosture int `json:"prep_posture"` // 0 оборона, 1 активная, 2 наступление
	WarPosture  int `json:"war_posture"`
	FortTiles   int `json:"fort_tiles"` // тайлов укреплять за раз
	// DefensePressure — суммарное давление противника на направлении, выше которого оно уходит в оборону.
	DefensePressure float64 `json:"defense_pressure"`
	// PostureDwellMin — не менять позицию направления чаще; DefenseTilePressure — среднее давление на тайл,
	// выше которого направление уходит в оборону (выходит при 60% от него); MainEveryMin — как часто пересматривать главный удар.
	PostureDwellMin     float64 `json:"posture_dwell_min"`
	DefenseTilePressure float64 `json:"defense_tile_pressure"`
	MainEveryMin        float64 `json:"main_every_min"`
	// Удары.
	StrikeEveryMin    float64            `json:"strike_every_min"`
	StrikeWeights     map[string]float64 `json:"strike_weights"`    // ценность целей по типу зданий
	StrikeMunitions   []string           `json:"strike_munitions"`  // порядок выбора боеприпасов
	SalvoBase         float64            `json:"salvo_base"`        // базовый залп
	SalvoPerChannel   float64            `json:"salvo_per_channel"` // добавка за каждый известный канал ПВО у цели
	MinSalvoFrac      float64            `json:"min_salvo_frac"`    // не бить слабее этой доли нужного залпа
	TargetCooldownMin float64            `json:"target_cooldown_min"`
	ReconEveryMin     float64            `json:"recon_every_min"`
	ReconStaleMin     float64            `json:"recon_stale_min"` // данные о цели старше — пора разведать
	// SalvoCommit — доля нужного залпа, которую обязательно собрать до пуска (иначе ждём накопления запасов);
	// SalvoHoldMin — за сколько минут ожидания требование ослабевает до 40%.
	// UnitStrikeWeights — ценность вражеских юнитов как целей по виду (ad, radar, launcher, reb), UnitFreshMin — насколько свежей
	// должна быть разведка о подвижной цели.
	UnitStrikeWeights map[string]float64 `json:"unit_strike_weights"`
	UnitFreshMin      float64            `json:"unit_fresh_min"`
	CoverPenalty      float64            `json:"cover_penalty"` // насколько каждый известный канал ПВО у цели снижает её приоритет (по умолчанию 0,15)
	SalvoCommit       float64            `json:"salvo_commit"`
	SalvoHoldMin      float64            `json:"salvo_hold_min"`
	HopelessSalvo     float64            `json:"hopeless_salvo"`     // залп, ниже которого не бьют, пока удары не долетают (по умолчанию 24)
	ReachFloor        float64            `json:"reach_floor"`        // доля долетевших ниже этой — удары «безнадёжны» (по умолчанию 0,1)
	ZeroCooldownMult  float64            `json:"zero_cooldown_mult"` // во сколько раз дольше не бить цель после двух провальных залпов (по умолчанию 3)
	// DecoyMunitions — приманки (запускаются вместе с залпом по цели под ПВО), DecoyPerChannel — штук на канал ПВО.
	DecoyMunitions  []string `json:"decoy_munitions"`
	DecoyPerChannel float64  `json:"decoy_per_channel"`
	// ПВО и ремонт.
	Protect         map[string]float64 `json:"protect"` // ценность своих зданий для прикрытия
	AdEveryMin      float64            `json:"ad_every_min"`
	AdMovesMax      int                `json:"ad_moves_max"`
	AdMoveCooldown  float64            `json:"ad_move_cooldown_min"`
	AdMoveMaxKm     float64            `json:"ad_move_max_km"`
	RepairMinWeight float64            `json:"repair_min_weight"` // здания дешевле — не чинить, пока ждут важные
}
