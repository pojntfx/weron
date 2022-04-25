package wrtcltc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/pojntfx/weron/pkg/services"
	"github.com/pojntfx/weron/pkg/wrtcconn"
)

type AdapterConfig struct {
	*wrtcconn.AdapterConfig
	OnSignalerConnect  func(string)
	OnPeerConnect      func(string)
	OnPeerDisconnected func(string)
}

type Totals struct {
	LatencyAverage time.Duration
	LatencyMin     time.Duration
	LatencyMax     time.Duration
	PacketsWritten int64
}

type Acknowledgement struct {
	BytesWritten int
	Latency      time.Duration
}

type Adapter struct {
	signaler string
	key      string
	ice      []string
	config   *AdapterConfig
	ctx      context.Context

	cancel  context.CancelFunc
	adapter *wrtcconn.Adapter

	ids              chan string
	totals           chan Totals
	acknowledgements chan Acknowledgement
}

func NewAdapter(
	signaler string,
	key string,
	ice []string,
	config *AdapterConfig,
	ctx context.Context,
) *Adapter {
	ictx, cancel := context.WithCancel(ctx)

	if config == nil {
		config = &AdapterConfig{}
	}

	return &Adapter{
		signaler: signaler,
		key:      key,
		ice:      ice,
		config:   config,
		ctx:      ictx,

		cancel: cancel,

		ids:              make(chan string),
		totals:           make(chan Totals),
		acknowledgements: make(chan Acknowledgement),
	}
}

func (a *Adapter) Open() error {
	a.adapter = wrtcconn.NewAdapter(
		a.signaler,
		a.key,
		strings.Split(strings.Join(a.ice, ","), ","),
		[]string{services.IPPrimary},
		a.config.AdapterConfig,
		a.ctx,
	)

	var err error
	a.ids, err = a.adapter.Open()
	if err != nil {
		return err
	}

	return err
}

func (a *Adapter) Close() error {
	return a.adapter.Close()
}

func (a *Adapter) Wait() error {
	return errors.New("unimplemented")
}

func (a *Adapter) Totals() chan Totals {
	return a.totals
}

func (a *Adapter) Acknowledgement() chan Acknowledgement {
	return a.acknowledgements
}
