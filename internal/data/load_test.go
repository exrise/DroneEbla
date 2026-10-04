package data

import "testing"

func TestLoadDefaults(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Buildings) == 0 || len(c.Units) == 0 || len(c.Objects) == 0 {
		t.Fatal("пустой каталог")
	}
}
