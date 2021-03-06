package game

import (
	"sync"

	"go.uber.org/atomic"
)

type Player struct {
	id          string
	name        string
	balance     atomic.Int64
	currentRoom *Room
	currentGame *Game

	mu sync.RWMutex
}

func NewPlayer(id string, name string) *Player {
	return &Player{id: id, name: name}
}

func (p *Player) ID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.id
}

func (p *Player) Name() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.name
}

func (p *Player) CurrentRoom() *Room {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentRoom
}

func (p *Player) CurrentGame() *Game {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentGame
}

func (p *Player) SetCurrentRoom(r *Room) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentRoom = r
}

func (p *Player) SetCurrentGame(g *Game) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentGame = g
}

func (p *Player) AddBalance(amount int64) {
	p.balance.Add(amount)
}

func (p *Player) Balance() int64 {
	return p.balance.Load()
}
