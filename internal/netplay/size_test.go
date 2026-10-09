package netplay

import (
	"bytes"
	"compress/flate"
	"encoding/gob"
	"testing"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
	"github.com/exrise/droneebla/internal/world"
)

func TestViewSize(t *testing.T) {
	cat, _ := data.Load("")
	m, _ := world.Load()
	w := sim.New(cat, m, false)
	for i := 0; i < 300; i++ {
		w.Step(1)
	}
	var buf bytes.Buffer
	fw, _ := flate.NewWriter(&buf, flate.BestSpeed)
	enc := gob.NewEncoder(fw)
	var last [4]uint32
	var sent bool
	v1 := w.BuildView(data.UA, 0)
	stripSameLayers(v1, &last, &sent)
	enc.Encode(&Msg{View: v1})
	fw.Flush()
	first := buf.Len()
	buf.Reset()
	w.Step(1)
	v2 := w.BuildView(data.UA, 1<<60)
	stripSameLayers(v2, &last, &sent)
	if v2.SameLayers&1 == 0 {
		t.Fatal("слой владельцев не менялся — должен быть пропущен")
	}
	enc.Encode(&Msg{View: v2})
	fw.Flush()
	t.Logf("первое сообщение %d КБ, следующее %d КБ (5 раз в секунду → %d КБ/с)", first/1024, buf.Len()/1024, buf.Len()*5/1024)
	if buf.Len() > 40*1024 {
		t.Fatal("слишком большое представление")
	}
}
