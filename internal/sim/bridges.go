package sim

// bridgeLink — мост, по которому техника проходит между двумя берегами: концы — тайлы суши.
type bridgeLink struct {
	id   uint32 // здание-мост
	a, b int    // тайлы концов
}

// bridgeLinks находит мосты с полем link в данных и привязывает их к зданиям и тайлам.
func (w *World) bridgeLinks() []bridgeLink {
	if w.bridgesOK {
		return w.bridges
	}
	w.bridges, w.bridgesOK = nil, true
	for _, o := range w.cat.Objects {
		if o.Type != "bridge" || len(o.Link) != 2 {
			continue
		}
		var id uint32
		for _, b := range w.Buildings {
			if b.Type == "bridge" && b.Name == o.Name && (id == 0 || b.ID < id) {
				id = b.ID
			}
		}
		if id == 0 {
			continue
		}
		x0, y0 := w.m.Project(o.Link[0][0], o.Link[0][1])
		x1, y1 := w.m.Project(o.Link[1][0], o.Link[1][1])
		a, b := w.tileOf(x0, y0), w.tileOf(x1, y1)
		if a < 0 || b < 0 {
			continue
		}
		w.bridges = append(w.bridges, bridgeLink{id, a, b})
	}
	return w.bridges
}

// bridgeOpen — мост цел настолько, что по нему проходят юниты стороны s, и оба берега свои.
func (w *World) bridgeOpen(s int, l bridgeLink) bool {
	b, ok := w.Buildings[l.id]
	if !ok || b.Side != s || b.Built < 1 || b.HP < b.MaxHP*w.cat.Rules.BridgePassFrac {
		return false
	}
	return w.OwnerSide(l.a) == s && w.OwnerSide(l.b) == s
}

// BridgeOpen — открыт ли для техники мост-здание b (для интерфейса и ИИ).
func (w *World) BridgeOpen(b *Building) bool {
	for _, l := range w.bridgeLinks() {
		if l.id == b.ID {
			return w.bridgeOpen(b.Side, l)
		}
	}
	return false
}

// deadBridgeAhead — следующий участок пути юнита идёт по мосту, который уже не пропускает технику.
func (w *World) deadBridgeAhead(u *Unit) bool {
	if len(u.Path) == 0 {
		return false
	}
	from, to := w.tileOf(u.X, u.Y), w.tileOf(u.Path[0].X, u.Path[0].Y)
	for _, l := range w.bridgeLinks() {
		if (from == l.a && to == l.b || from == l.b && to == l.a) && !w.bridgeOpen(u.Side, l) {
			return true
		}
	}
	return false
}
