package game

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"
	"go.uber.org/atomic"
)

type Manager struct {
	maxBet atomic.Uint64

	players sync.Map
	rooms   sync.Map
	games   sync.Map
	mu      sync.RWMutex

	onNewRoomFunc        OnNewRoomFunc
	onNewGameFunc        OnNewGameFunc
	onPlayerJoinRoomFunc OnPlayerJoinRoomFunc
	onPlayerBetFunc      OnPlayerBetFunc
	onPlayerStandFunc    OnPlayerStandFunc
	onPlayerHitFunc      OnPlayerHitFunc
	onGameFinishFunc     OnGameFinishFunc
	onPlayerPlayFunc     OnPlayerPlayFunc
}

type OnNewGameFunc func(r *Room, g *Game)
type OnNewRoomFunc func(r *Room, creator *Player)
type OnPlayerJoinRoomFunc func(r *Room, p *Player)
type OnPlayerBetFunc func(g *Game, p *PlayerInGame)
type OnPlayerStandFunc func(g *Game, p *PlayerInGame)
type OnPlayerHitFunc func(g *Game, p *PlayerInGame)
type OnGameFinishFunc func(g *Game)
type OnPlayerPlayFunc func(g *Game, pg *PlayerInGame)

func NewManager(maxBet uint64) *Manager {
	m := &Manager{
		maxBet: *atomic.NewUint64(maxBet),
	}
	return m
}

func (m *Manager) PlayerRegister(ctx context.Context, id string, name string) *Player {
	pp, ok := m.players.Load(id)
	if !ok || pp == nil {
		pp = NewPlayer(id, name)
		m.mu.Lock()
		m.players.Store(id, pp)
		m.mu.Unlock()
		log.Ctx(ctx).Debug().Msg("player start using bot")
	}
	return pp.(*Player)
}

func (m *Manager) NewRoom(ctx context.Context, p *Player) (*Room, error) {
	if p.CurrentRoom() != nil {
		return nil, ErrYouAlreadyInAnotherRoom
	}

	var id string
	for {
		id = generateRoomID()
		if _, exist := m.rooms.Load(id); !exist {
			break
		}
	}

	r := NewRoom(id, p)
	p.SetCurrentRoom(r)
	log.Ctx(ctx).Debug().Str("room", r.ID()).Msg("new room created")

	m.rooms.Store(r.ID(), r)

	m.mu.RLock()
	f := m.onNewRoomFunc
	m.mu.RUnlock()

	if f != nil {
		f(r, p)
	}
	return r, nil
}

func (m *Manager) FindPlayer(ctx context.Context, id string) *Player {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getPlayer(ctx, id)
}

func (m *Manager) FindRoom(ctx context.Context, roomID string) *Room {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getRoom(ctx, roomID)
}

func (m *Manager) Players() []*Player {
	var ps []*Player
	m.players.Range(func(id, pp interface{}) bool {
		ps = append(ps, pp.(*Player))
		return true
	})
	return ps
}

func (m *Manager) JoinRoom(ctx context.Context, p *Player, r *Room) error {
	if cr := p.CurrentRoom(); cr != nil {
		if cr.ID() == r.ID() {
			return ErrYouAlreadyInRoom
		}
		return ErrYouAlreadyInAnotherRoom
	}

	if err := r.JoinPlayer(p); err != nil {
		return err
	}
	p.SetCurrentRoom(r)

	m.mu.RLock()
	f := m.onPlayerJoinRoomFunc
	m.mu.RUnlock()
	if f != nil {
		f(r, p)
	}
	log.Ctx(ctx).Debug().Str("room", r.ID()).Msg("player joined room")
	return nil
}

func (m *Manager) LeaveRoom(ctx context.Context, p *Player) (*Room, error) {
	if p.CurrentGame() != nil {
		return nil, ErrYouAlreadyInGame
	}
	if p.CurrentRoom() == nil {
		return nil, ErrNotInRoom
	}

	r := p.CurrentRoom()
	r.RemovePlayer(p)
	if len(r.Players()) == 0 {
		m.rooms.Delete(r.ID())
	}
	p.SetCurrentRoom(nil)
	log.Ctx(ctx).Debug().Str("room", r.ID()).Msg("player left room")
	return r, nil
}

func (m *Manager) OnNewRoom(f OnNewRoomFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onNewRoomFunc = f
}

func (m *Manager) OnNewGame(f OnNewGameFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onNewGameFunc = f
}

func (m *Manager) OnPlayerJoinRoom(f OnPlayerJoinRoomFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPlayerJoinRoomFunc = f
}

func (m *Manager) OnPlayerBet(f OnPlayerBetFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPlayerBetFunc = f
}

func (m *Manager) OnPlayerStand(f OnPlayerStandFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPlayerStandFunc = f
}

func (m *Manager) OnGameFinish(f OnGameFinishFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onGameFinishFunc = f
}

func (m *Manager) OnPlayerHit(f OnPlayerHitFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPlayerHitFunc = f
}

func (m *Manager) OnPlayerPlay(f OnPlayerPlayFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPlayerPlayFunc = f
}

func (m *Manager) getPlayer(ctx context.Context, id string) *Player {
	p, ok := m.players.Load(id)
	if !ok || p == nil {
		return nil
	}
	return p.(*Player)
}

func (m *Manager) getRoom(ctx context.Context, id string) *Room {
	r, ok := m.rooms.Load(id)
	if !ok || r == nil {
		return nil
	}
	return r.(*Room)
}

func (m *Manager) NewGame(room *Room, dealer *Player) (*Game, error) {
	if room.CurrentGame() != nil {
		return nil, ErrGameIsExisted
	}

	g := NewGame(dealer, room, m.maxBet.Load())
	dealer.SetCurrentGame(g)
	room.SetCurrentGame(g)
	m.games.Store(g.ID(), g)

	m.mu.RLock()
	f := m.onNewGameFunc
	m.mu.RUnlock()

	if f != nil {
		f(room, g)
	}
	return g, nil
}

func (m *Manager) FindGame(ctx context.Context, gameID string) *Game {
	p, ok := m.games.Load(gameID)
	if !ok || p == nil {
		return nil
	}
	return p.(*Game)
}

func (m *Manager) PlayerBet(ctx context.Context, g *Game, p *Player, amount uint64) (err error) {
	var pg *PlayerInGame
	if amount == 0 {
		if err = g.RemovePlayer(p.ID()); err != nil {
			return err
		}
	} else {
		pg, err = g.PlayerBet(p, amount)
		if err != nil {
			return err
		}
		p.SetCurrentGame(g)
	}

	m.mu.RLock()
	f := m.onPlayerBetFunc
	m.mu.RUnlock()
	if f != nil {
		f(g, pg)
	}

	return nil
}

func (m *Manager) PlayerStand(ctx context.Context, g *Game, pg *PlayerInGame) error {
	if pg.IsDone() {
		return nil
	}
	if !pg.CanStand() {
		return ErrYouCannotStand
	}
	if err := g.PlayerStand(pg); err != nil {
		log.Ctx(ctx).Err(err).Str("cards", pg.Cards().String(false)).Msg("player stand failed")
		return err
	}

	m.mu.RLock()
	f := m.onPlayerStandFunc
	m.mu.RUnlock()

	if f != nil {
		f(g, pg)
	}

	if _, err := g.PlayerNext(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) PlayerHit(ctx context.Context, g *Game, pg *PlayerInGame) error {
	if !pg.CanHit() {
		return ErrYouCannotHit
	}

	c, err := g.RemoveCard()
	if err != nil {
		return err
	}
	pg.AddCard(c)

	m.mu.RLock()
	f := m.onPlayerHitFunc
	m.mu.RUnlock()

	if f != nil {
		f(g, pg)
	}
	return nil
}

func (m *Manager) CheckIfFinish(ctx context.Context, g *Game) bool {
	if !g.Finished() {
		return false
	}
	err := m.FinishGame(ctx, g, false)
	return err == nil
}

func (m *Manager) Deal(ctx context.Context, g *Game) error {
	g.OnPlayerPlay(func(pg *PlayerInGame) {
		m.mu.RLock()
		f := m.onPlayerPlayFunc
		m.mu.RUnlock()
		if f != nil {
			f(g, pg)
		}
	})
	if err := g.Deal(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) Start(ctx context.Context, g *Game) error {
	// check for early win
	gt := g.Dealer().Type()
	if gt == TypeDoubleBlackJack || gt == TypeBlackJack {
		return m.FinishGame(ctx, g, true)
	}

	cnt := 0
	for _, p := range g.Players() {
		pt := p.Type()
		if pt == TypeDoubleBlackJack || pt == TypeBlackJack {
			_, _ = g.Done(p, true)
			cnt++
		}
	}

	if cnt == len(g.Players()) {
		return m.FinishGame(ctx, g, true)
	}

	if _, err := g.PlayerNext(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) FinishGame(ctx context.Context, g *Game, force bool) error {
	for _, pg := range g.Players() {
		if _, err := g.Done(pg, force); err != nil {
			return err
		}
	}

	g.Dealer().SetCurrentGame(nil)
	for _, p := range g.Players() {
		p.Player.AddBalance(p.Reward())
		p.SetCurrentGame(nil)
	}
	g.Dealer().Player.AddBalance(g.Dealer().Reward())
	m.games.Delete(g.ID())
	r := g.Room()
	r.SetCurrentGame(nil)

	m.mu.RLock()
	f := m.onGameFinishFunc
	m.mu.RUnlock()
	if f != nil {
		f(g)
	}
	return nil
}

func (m *Manager) CancelGame(ctx context.Context, g *Game) error {
	if g.Status() != Betting {
		return ErrGameAlreadyStarted
	}
	for _, pg := range g.Players() {
		pg.Player.SetCurrentGame(nil)
	}
	g.Room().SetCurrentGame(nil)
	m.games.Delete(g.ID())
	return nil
}

func (m *Manager) Rooms(ctx context.Context) ([]*Room, error) {
	var rs []*Room
	m.rooms.Range(func(id, rr interface{}) bool {
		r := rr.(*Room)
		rs = append(rs, r)
		return true
	})
	return rs, nil
}

func (m *Manager) SetMaxBet(maxBet uint64) uint64 {
	m.maxBet.Store(maxBet)
	return maxBet
}
