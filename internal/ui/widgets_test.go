package ui

import (
	"testing"

	"github.com/exrise/droneebla/internal/sim"
)

func TestTextFieldInput(t *testing.T) {
	f := TextField{Max: 15, Allowed: func(r rune) bool { return r != ' ' }}
	f.applyInput([]rune("26"), 0)
	f.applyInput([]rune{'.', ' ', '1', '\n', 0x7f}, 0)
	if f.Text != "26.1" {
		t.Fatalf("получили %q, ожидали 26.1", f.Text)
	}
	f.applyInput(nil, 2)
	if f.Text != "26" {
		t.Fatalf("Backspace: получили %q", f.Text)
	}
	f.applyInput([]rune("1234567890123456"), 0)
	if len([]rune(f.Text)) != 15 {
		t.Fatalf("ограничение длины не сработало: %q", f.Text)
	}
}

// Ввод одного тика не должен попадать в текст повторно, сколько бы раз ни
// вызвали Draw до следующего Update (монитор 120/144 Гц).
func TestTextInputNotDuplicatedByDraw(t *testing.T) {
	g := &Game{}
	f := TextField{Focused: true}
	g.ui.in.chars = append(g.ui.in.chars, '5') // как после Update
	for frame := 0; frame < 3; frame++ {       // три кадра Draw на один тик
		f.applyInput(g.ui.in.chars, g.ui.in.backspace)
		g.ui.in.chars, g.ui.in.backspace = nil, 0
		g.resetInput()
	}
	if f.Text != "5" {
		t.Fatalf("цифра продублирована: %q", f.Text)
	}
	// Без потребления полем resetInput всё равно очищает ввод.
	g.ui.in.chars = append(g.ui.in.chars, '7')
	g.ui.in.backspace = 2
	g.resetInput()
	if len(g.ui.in.chars) != 0 || g.ui.in.backspace != 0 {
		t.Fatal("resetInput не очистил ввод")
	}
}

func TestControlGroups(t *testing.T) {
	g := &Game{view: &sim.View{Units: []sim.Unit{{ID: 1, X: 10, Y: 20}, {ID: 2, X: 30, Y: 40}}, Buildings: []sim.Building{{ID: 7, X: 5, Y: 6}}}}
	g.sel = Selection{Kind: "unit", ID: 1}
	g.setGroup(3, false)
	g.sel = Selection{Kind: "unit", ID: 2}
	g.setGroup(3, true)
	g.setGroup(3, true) // повторное добавление не плодит дубль
	g.sel = Selection{Kind: "building", ID: 7}
	g.setGroup(3, true)
	if m := g.groupMembers(3); len(m) != 3 {
		t.Fatalf("в группе %d членов, ожидалось 3", len(m))
	}
	g.sel = Selection{Kind: "unit", ID: 2}
	g.setGroup(4, false) // Ctrl+цифра заменяет группу
	g.sel = Selection{Kind: "unit", ID: 1}
	g.setGroup(4, false)
	if m := g.groupMembers(4); len(m) != 1 || m[0].sel.ID != 1 {
		t.Fatalf("группа 4: %+v", m)
	}
	// Погибший юнит выпадает из группы.
	g.view.Units = g.view.Units[1:]
	if m := g.groupMembers(3); len(m) != 2 {
		t.Fatalf("после гибели юнита 1 в группе %d, ожидалось 2", len(m))
	}
	// Контакты противника в группы не записываются.
	g.sel = Selection{Kind: "contact", ID: 9}
	g.setGroup(5, false)
	if m := g.groupMembers(5); len(m) != 0 {
		t.Fatal("контакт попал в группу")
	}
}
