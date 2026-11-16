// Package omap is an insertion-ordered map, the Go stand-in for a Ruby Hash where the Ruby
// reference relies on the order keys were first added (reports list them in that order).
package omap

// Map keeps its keys in the order they were first set.
type Map[K comparable, V any] struct {
	keys   []K
	values map[K]V
}

// New returns an empty map.
func New[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{values: map[K]V{}}
}

// Set stores a value; a new key goes last, an existing one keeps its place.
func (m *Map[K, V]) Set(key K, value V) {
	if _, ok := m.values[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

// Get returns a value and whether the key is present.
func (m *Map[K, V]) Get(key K) (V, bool) {
	value, ok := m.values[key]
	return value, ok
}

// Has reports whether the key is present.
func (m *Map[K, V]) Has(key K) bool {
	_, ok := m.values[key]
	return ok
}

// Delete removes a key; the others keep their order.
func (m *Map[K, V]) Delete(key K) {
	if _, ok := m.values[key]; !ok {
		return
	}
	delete(m.values, key)
	for index, candidate := range m.keys {
		if candidate == key {
			m.keys = append(m.keys[:index], m.keys[index+1:]...)
			return
		}
	}
}

// Keys returns the keys in order (a copy).
func (m *Map[K, V]) Keys() []K {
	return append([]K(nil), m.keys...)
}

// Len is the number of keys.
func (m *Map[K, V]) Len() int { return len(m.keys) }

// First returns the earliest key still present.
func (m *Map[K, V]) First() (K, V, bool) {
	if len(m.keys) == 0 {
		var key K
		var value V
		return key, value, false
	}
	key := m.keys[0]
	return key, m.values[key], true
}

// Each calls fn for every key and value in order.
func (m *Map[K, V]) Each(fn func(K, V)) {
	for _, key := range m.keys {
		fn(key, m.values[key])
	}
}
