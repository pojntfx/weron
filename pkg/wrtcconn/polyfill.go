package wrtcconn

import (
	"context"
	"net"

	"nhooyr.io/websocket"
)

type wrapWSConn struct {
	*websocket.Conn
	config *AdapterConfig
	raddr  WSRmote
}

func newWrapWSConn(conn *websocket.Conn, config *AdapterConfig, host string) *wrapWSConn {
	return &wrapWSConn{
		Conn:   conn,
		config: config,
		raddr:  WSRmote(host),
	}
}

func (w *wrapWSConn) ReadMessage() (websocket.MessageType, []byte, error) {
	return w.Conn.Read(context.Background())
}

func (w *wrapWSConn) WriteMessage(t websocket.MessageType, p []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), w.config.Timeout)
	defer cancel()
	return w.Conn.Write(ctx, t, p)
}

func (w *wrapWSConn) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), w.config.Timeout)
	defer cancel()
	return w.Conn.Ping(ctx)
}

func (w *wrapWSConn) RemoteAddr() net.Addr {
	return w.raddr
}

type WSRmote string

var _ net.Addr = (*WSRmote)(nil)

func (WSRmote) Network() string     { return "ws" }
func (host WSRmote) String() string { return string(host) }
