package world

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// TestPreview сохраняет картинку карты, если задана переменная MAP_PREVIEW.
func TestPreview(t *testing.T) {
	path := os.Getenv("MAP_PREVIEW")
	if path == "" {
		t.Skip("MAP_PREVIEW не задан")
	}
	m, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	s := 3
	img := image.NewRGBA(image.Rect(0, 0, m.W*s, m.H*s))
	for ty := 0; ty < m.H; ty++ {
		for tx := 0; tx < m.W; tx++ {
			i := m.Idx(tx, ty)
			c := color.RGBA{150, 180, 210, 255}
			if m.Terrain[i] == TerrainLand {
				switch m.Initial[i] {
				case 1:
					c = color.RGBA{220, 170, 160, 255}
				case 2:
					c = color.RGBA{170, 190, 230, 255}
				default:
					c = color.RGBA{200, 200, 190, 255}
				}
				if m.Country[i] == CountryBelarus {
					c = color.RGBA{210, 200, 170, 255}
				}
				if m.Flags[i]&FlagRiver != 0 {
					c = color.RGBA{60, 100, 200, 255}
				}
				if m.Deposit[i] != 0 {
					c.G -= 40
				}
				if m.Flags[i]&FlagUrban != 0 {
					c = color.RGBA{80, 80, 80, 255}
				}
			}
			for y := 0; y < s; y++ {
				for x := 0; x < s; x++ {
					img.Set(tx*s+x, ty*s+y, c)
				}
			}
		}
	}
	f, _ := os.Create(path)
	defer f.Close()
	png.Encode(f, img)
}
