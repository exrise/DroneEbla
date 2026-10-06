package ui

import "testing"

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
