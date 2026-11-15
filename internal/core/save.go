package core

// Save/load: JSON schema v1 в конверте {schema, blocks, links, taps} (ADR-0005).
// Реализовано как MarshalJSON/UnmarshalJSON у Document. Загрузка атомарна:
// невалидный файл не портит документ. Разбор — docs/ALGORITHMS.md §8.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// SchemaVersion — версия формата. Ломающие изменения повышают её и мигрируют.
const SchemaVersion = 1

type fileV1 struct {
	Schema int      `json:"schema"`
	Blocks []*Block `json:"blocks"`
	Links  []Link   `json:"links"`
	// Taps — T-соединения (введены 2026-11-08, идея 1.3). Необязательное
	// поле: файлы без него валидны, схема остаётся v1 (расширение, не ломание).
	Taps []TapLink `json:"taps,omitempty"`
}

// MarshalJSON — сериализация документа (schema v1).
func (d *Document) MarshalJSON() ([]byte, error) {
	return json.MarshalIndent(fileV1{
		Schema: SchemaVersion,
		Blocks: d.Blocks,
		Links:  d.Links,
		Taps:   d.Taps,
	}, "", "  ")
}

// UnmarshalJSON — загрузка документа: валидация + продолжение нумерации ID.
func (d *Document) UnmarshalJSON(data []byte) error {
	var f fileV1
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	if f.Schema != SchemaVersion {
		return fmt.Errorf("schema v%d не поддерживается (ожидается v%d)", f.Schema, SchemaVersion)
	}
	nd := &Document{Blocks: f.Blocks, Links: f.Links, Taps: f.Taps}
	if err := nd.Validate(); err != nil {
		return err
	}
	nd.seq = nextSeq(nd)
	*d = *nd // атомарно: доктину заменяем только после валидации
	return nil
}

// Validate — структурная целостность: уникальные непустые ID блоков,
// осмысленные размеры, все endpoints линков живые.
func (d *Document) Validate() error {
	ids := map[string]bool{}
	for i, b := range d.Blocks {
		if b == nil {
			return fmt.Errorf("block %d: nil", i)
		}
		if b.ID == "" {
			return fmt.Errorf("block %d: пустой ID", i)
		}
		if ids[b.ID] {
			return fmt.Errorf("дубликат блока %q", b.ID)
		}
		ids[b.ID] = true
		if b.Pins < 0 || b.W < 1 || b.H < 1 {
			return fmt.Errorf("block %q: pins=%d w=%v h=%v некорректны", b.ID, b.Pins, b.W, b.H)
		}
	}
	for i, l := range d.Links {
		if !d.EndpointValid(l.From) {
			return fmt.Errorf("link %d: from %s не существует", i, l.From.String())
		}
		if !d.EndpointValid(l.To) {
			return fmt.Errorf("link %d: to %s не существует", i, l.To.String())
		}
	}
	for i, t := range d.Taps {
		if !d.EndpointValid(t.From) {
			return fmt.Errorf("tap %d: from %s не существует", i, t.From.String())
		}
		if !t.Tap.FractionValid() {
			return fmt.Errorf("tap %d: доля t=%v вне 0..1", i, t.Tap.T)
		}
		host, ok := d.LinkByKey(t.Tap.Host)
		if !ok {
			return fmt.Errorf("tap %d: линк-хозяин %q не существует", i, t.Tap.Host)
		}
		if host.From == t.From || host.To == t.From {
			return fmt.Errorf("tap %d: хозяин %q идёт из того же пина", i, t.Tap.Host)
		}
	}
	return nil
}

// nextSeq — продолжение автонумерации «bN» после загрузки (max N среди ID).
func nextSeq(d *Document) int {
	max := 0
	for _, b := range d.Blocks {
		if n, err := strconv.Atoi(strings.TrimPrefix(b.ID, "b")); err == nil && n > max {
			max = n
		}
	}
	return max
}

// RemoveBlock удаляет блок и все его линки. false — блок не найден.
func (d *Document) RemoveBlock(id string) bool {
	at := -1
	for i, b := range d.Blocks {
		if b.ID == id {
			at = i
			break
		}
	}
	if at < 0 {
		return false
	}
	d.Blocks = append(d.Blocks[:at], d.Blocks[at+1:]...)
	links := d.Links[:0]
	for _, l := range d.Links {
		if l.From.BlockID != id && l.To.BlockID != id {
			links = append(links, l)
		}
	}
	d.Links = links[:len(links):len(links)] // отсечение backing-остатков
	return true
}
