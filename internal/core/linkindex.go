package core

// Доступ к линкам по ключу: T-соединения адресуют хозяина строкой
// «блок.пин->блок.пин». Поиск линейный — документы этапа 1 маленькие.

// LinkByKey — линк по каноническому ключу From → To (LinkKey).
func (d *Document) LinkByKey(k string) (Link, bool) {
	i := d.LinkIndex(k)
	if i < 0 {
		return Link{}, false
	}
	return d.Links[i], true
}

// LinkIndex — индекс линка по ключу, -1 если нет.
func (d *Document) LinkIndex(k string) int {
	for i, l := range d.Links {
		if LinkKey(l) == k {
			return i
		}
	}
	return -1
}

// TapHostOf — ключ линка, на котором лежит T-соединение из пина from
// (пустая строка — такого соединения нет).
func (d *Document) TapHostOf(from Endpoint) string {
	for _, t := range d.Taps {
		if t.From == from {
			return t.Tap.Host
		}
	}
	return ""
}
