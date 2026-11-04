package core

import (
	"encoding/json"
	"testing"
)

// Раундтрип: поля блоков, линки, junction, автонумерация ID после загрузки.
func TestSaveLoadRoundtrip(t *testing.T) {
	d := &Document{}
	d.AddBlock(&Block{ID: "A", Kind: "src", X: 10, Y: 20, W: 120, H: 64, Label: "Блок-А", Pins: 4})
	d.AddBlock(&Block{ID: "b3", X: 300, Y: 200, W: 100, H: 60, Pins: 2})
	d.AddBlock(&Block{ID: "b11", X: 500, Y: 40, Pins: 6}) // размеры дыфолтятся в AddBlock
	d.AddLink(Endpoint{"A", 0}, Endpoint{"b3", 0})
	d.AddLink(Endpoint{"A", 0}, Endpoint{"b3", 1}) // junction на A.0
	d.AddLink(Endpoint{"b11", 5}, Endpoint{"b3", 1})

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var nd Document
	if err := json.Unmarshal(data, &nd); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(nd.Blocks) != 3 || len(nd.Links) != 3 {
		t.Fatalf("roundtrip состав: blocks=%d links=%d", len(nd.Blocks), len(nd.Links))
	}
	nb := nd.BlockByID("A")
	if nb == nil || nb.Kind != "src" || nb.X != 10 || nb.Y != 20 ||
		nb.W != 120 || nb.H != 64 || nb.Pins != 4 || nb.Label != "Блок-А" {
		t.Fatalf("поля блока тянутся: %+v", nb)
	}
	if !nd.EndpointValid(Endpoint{"b11", 5}) {
		t.Fatal("пин 5 у 6-пинового блока должен существовать")
	}
	// автонумерация продолжается после загрузки (макс b11 → новый b12)
	nb2 := nd.AddBlock(&Block{X: 0, Y: 0})
	if nb2.ID != "b12" {
		t.Fatalf("после load автонумерация должна дать b12, получили %q", nb2.ID)
	}
}

// Неподдерживаемая версия схемы отвергается.
func TestLoadRejectsBadSchema(t *testing.T) {
	var d Document
	err := json.Unmarshal([]byte(`{"schema": 7, "blocks": []}`), &d)
	if err == nil {
		t.Fatal("schema 7 должен быть отвергнут")
	}
}

// Висячий линк (нет такого блока) отвергается валидацией.
func TestLoadRejectsDanglingLink(t *testing.T) {
	var d Document
	bad := `{"schema":1,"blocks":[{"id":"a","x":0,"y":0,"w":120,"h":64,"pins":2}],` +
		`"links":[{"from":{"block":"ghost","pin":0},"to":{"block":"a","pin":0}}]}`
	if err := json.Unmarshal([]byte(bad), &d); err == nil {
		t.Fatal("висячий линк должен упасть валидацией")
	}
}

// Дубликаты ID блоков отвергаются.
func TestLoadRejectsDuplicateIDs(t *testing.T) {
	var d Document
	bad := `{"schema":1,"blocks":[{"id":"a","x":0,"y":0,"w":120,"h":64,"pins":2},` +
		`{"id":"a","x":10,"y":0,"w":120,"h":64,"pins":2}],"links":[]}`
	if err := json.Unmarshal([]byte(bad), &d); err == nil {
		t.Fatal("дубликат ID должен упасть валидацией")
	}
}

// Атомарность: ошибочный файл не заменяет существующий документ.
func TestLoadAtomic(t *testing.T) {
	d := &Document{}
	d.AddBlock(&Block{ID: "a", X: 1, Y: 2, W: 120, H: 64, Pins: 2})
	bad := `{"schema":1,"blocks":[{"id":"z","x":0,"y":0,"w":120,"h":64,"pins":2}],` +
		`"links":[{"from":{"block":"zzz","pin":0},"to":{"block":"z","pin":0}}]}`
	if err := json.Unmarshal([]byte(bad), d); err == nil {
		t.Fatal("ожидали ошибку валидации")
	}
	b := d.BlockByID("a")
	if b == nil || b.X != 1 || b.Y != 2 {
		t.Fatalf("документ заменён при ошибке: %+v", b)
	}
}

// RemoveBlock убирает блок и его линки.
func TestRemoveBlock(t *testing.T) {
	d := &Document{}
	a := d.AddBlock(&Block{ID: "A", X: 0, Y: 0, W: 120, H: 64, Pins: 2})
	b := d.AddBlock(&Block{ID: "B", X: 200, Y: 0, W: 120, H: 64, Pins: 2})
	d.AddLink(Endpoint{"A", 0}, Endpoint{"B", 0})

	if !d.RemoveBlock("A") {
		t.Fatal("RemoveBlock=false")
	}
	if d.RemoveBlock("A") {
		t.Fatal("двойное удаление прошло")
	}
	if len(d.Blocks) != 1 || len(d.Links) != 0 {
		t.Fatalf("после удаления: blocks=%d links=%d", len(d.Blocks), len(d.Links))
	}
	if b.X != 200 || d.BlockByID("B") != b {
		t.Fatal("задел другой блок")
	}
	_ = a
}
