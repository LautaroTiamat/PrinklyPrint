package server

import (
	"sync"
	"time"
)

// tokenBucket es un limitador token-bucket threadsafe, hecho a mano (sin deps).
//
// Modela una tasa sostenida con tolerancia a ráfagas: el bucket tiene capacidad
// `capacity` (el burst) y se recarga `refillPerSec` tokens por segundo. Cada
// Allow() consume un token si hay; si no, devuelve false. El burst deja pasar
// picos legítimos; la tasa modera el ritmo sostenido.
//
// El reloj es inyectable (now) para tests deterministas.
type tokenBucket struct {
	mu           sync.Mutex
	capacity     float64
	tokens       float64
	refillPerSec float64
	last         time.Time
	now          func() time.Time
}

// newTokenBucket crea un bucket lleno (arranca con `burst` tokens). perMinute es
// la tasa sostenida; burst la capacidad. now puede ser nil (usa time.Now).
func newTokenBucket(perMinute, burst int, now func() time.Time) *tokenBucket {
	if now == nil {
		now = time.Now
	}
	return &tokenBucket{
		capacity:     float64(burst),
		tokens:       float64(burst),
		refillPerSec: float64(perMinute) / 60.0,
		last:         now(),
		now:          now,
	}
}

// reconfigure ajusta tasa/burst en vivo (lectura en vivo de la config). Si no
// cambiaron, es no-op. Si cambió la capacidad, recorta los tokens al nuevo tope.
func (b *tokenBucket) reconfigure(perMinute, burst int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	capacity := float64(burst)
	rate := float64(perMinute) / 60.0
	if b.capacity == capacity && b.refillPerSec == rate {
		return
	}
	b.capacity = capacity
	b.refillPerSec = rate
	if b.tokens > capacity {
		b.tokens = capacity
	}
}

// allow consume un token. Devuelve true si había uno disponible (request
// permitido), false si el bucket está vacío (request a limitar).
func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens += elapsed * b.refillPerSec
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
