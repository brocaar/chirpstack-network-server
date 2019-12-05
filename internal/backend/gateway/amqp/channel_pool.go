package amqp

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/streadway/amqp"
)

var errClosed = errors.New("pool is closed")

type poolChannel struct {
	ch       *amqp.Channel
	mu       sync.RWMutex
	p        *pool
	unusable bool
}

type pool struct {
	mu    sync.RWMutex
	chans chan *amqp.Channel
	conn  *amqp.Connection
}

func newPool(size int, conn *amqp.Connection) (*pool, error) {
	p := &pool{
		chans: make(chan *amqp.Channel, size),
		conn:  conn,
	}

	for i := 0; i < size; i++ {
		ch, err := conn.Channel()
		if err != nil {
			p.close()
			return nil, errors.Wrap(err, "create channel error")
		}

		p.chans <- ch
	}

	return p, nil
}

func (p *pool) getChansAndConn() (chan *amqp.Channel, *amqp.Connection) {
	p.mu.RLock()
	chans := p.chans
	conn := p.conn
	p.mu.RUnlock()
	return chans, conn
}

func (p *pool) wrapChan(ch *amqp.Channel) *poolChannel {
	return &poolChannel{
		ch: ch,
		p:  p,
	}
}

func (p *pool) get() (*poolChannel, error) {
	chans, conn := p.getChansAndConn()
	if chans == nil {
		return nil, errClosed
	}

	select {
	case ch := <-chans:
		if ch == nil {
			return nil, errors.New("channel is closed")
		}

		return p.wrapChan(ch), nil
	default:
		ch, err := conn.Channel()
		if err != nil {
			return nil, err
		}

		return p.wrapChan(ch), nil
	}
}

func (p *pool) put(ch *amqp.Channel) error {
	if ch == nil {
		return errors.New("channel is nil, rejecting")
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.chans == nil {
		return ch.Close()
	}

	select {
	case p.chans <- ch:
		return nil
	default:
		return ch.Close()
	}
}

func (p *pool) close() {
	p.mu.Lock()
	chans := p.chans
	p.chans = nil
	p.conn = nil
	p.mu.Unlock()

	if chans == nil {
		return
	}

	close(chans)
	for ch := range chans {
		ch.Close()
	}
}

func (pc *poolChannel) close() error {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.unusable {
		if pc.ch != nil {
			return pc.ch.Close()
		}
		return nil
	}

	return pc.p.put(pc.ch)
}

func (pc *poolChannel) markUnusable() {
	pc.mu.Lock()
	pc.unusable = true
	pc.mu.Unlock()
}
