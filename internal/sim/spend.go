package sim

// Категории расходов денег для разбивки в интерфейсе.
const (
	SpendOrders  = "orders"  // госзаказ
	SpendBuild   = "build"   // стройка
	SpendImport  = "import"  // закупки за рубежом
	SpendRepair  = "repair"  // ремонт
	SpendScience = "science" // финансирование исследований
	SpendPlants  = "plants"  // деньги, которые тратят сами заводы (завод электроники)
	SpendOther   = "other"   // пропаганда, укрепления, маскировка, мобилизация, агентура, подписка
)

// SpendCats — порядок категорий в интерфейсе.
var SpendCats = []string{SpendOrders, SpendBuild, SpendImport, SpendRepair, SpendPlants, SpendScience, SpendOther}

// spend записывает расход денег стороны s по категории (накапливается за шаг, затем сглаживается в час).
func (w *World) spend(s int, cat string, amount float64) {
	if amount <= 0 {
		return
	}
	sd := w.Sides[s]
	if sd.spendStep == nil {
		sd.spendStep = map[string]float64{}
	}
	sd.spendStep[cat] += amount
}

// smoothSpend переводит накопленные за шаг расходы в сглаженный расход в час (окно около 2 игровых часов).
func (w *World) smoothSpend(s int, dtH float64) {
	if dtH <= 0 {
		return
	}
	sd := w.Sides[s]
	if sd.SpendRate == nil {
		sd.SpendRate = map[string]float64{}
	}
	if sd.spendStep == nil {
		sd.spendStep = map[string]float64{}
	}
	a := dtH / 2
	if a > 1 {
		a = 1
	}
	for _, c := range SpendCats {
		sd.SpendRate[c] += (sd.spendStep[c]/dtH - sd.SpendRate[c]) * a
		sd.spendStep[c] = 0
	}
}
