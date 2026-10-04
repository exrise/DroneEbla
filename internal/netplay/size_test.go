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
	enc.Encode(&Msg{View: w.BuildView(data.UA, 0)})
	fw.Flush()
	first := buf.Len()
	buf.Reset()
	enc.Encode(&Msg{View: w.BuildView(data.UA, 1<<60)})
	fw.Flush()
	t.Logf("первое сообщение %d КБ, следующее %d КБ (5 раз в секунду → %d КБ/с)", first/1024, buf.Len()/1024, buf.Len()*5/1024)
	if buf.Len() > 200*1024 {
		t.Fatal("слишком большое представление")
	}
}
