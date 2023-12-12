package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	json "github.com/KpnmServer/go-util/json"
	"github.com/gorilla/websocket"
)

type EPacketType int8

const (
	EP_UNKNOWN EPacketType = -1
	EP_OPEN    EPacketType = iota
	EP_CLOSE
	EP_PING
	EP_PONG
	EP_MESSAGE
)

func (t EPacketType) String() string {
	switch t {
	case EP_OPEN:
		return "OPEN"
	case EP_CLOSE:
		return "CLOSE"
	case EP_PING:
		return "PING"
	case EP_PONG:
		return "PONG"
	case EP_MESSAGE:
		return "MESSAGE"
	}
	return "UNKNOWN"
}

func (t EPacketType) ID() string {
	switch t {
	case EP_OPEN:
		return "0"
	case EP_CLOSE:
		return "1"
	case EP_PING:
		return "2"
	case EP_PONG:
		return "3"
	case EP_MESSAGE:
		return "4"
	}
	return "-"
}

func ParseEPT(id string) EPacketType {
	if len(id) != 1 {
		return EP_UNKNOWN
	}
	switch id {
	case "0":
		return EP_OPEN
	case "1":
		return EP_CLOSE
	case "2":
		return EP_PING
	case "3":
		return EP_PONG
	case "4":
		return EP_MESSAGE
	}
	return EP_UNKNOWN
}

func GetEPT(id byte) EPacketType {
	switch id {
	case 0:
		return EP_OPEN
	case 1:
		return EP_CLOSE
	case 2:
		return EP_PING
	case 3:
		return EP_PONG
	case 4:
		return EP_MESSAGE
	}
	return EP_UNKNOWN
}

type SPacketType int8

const (
	SP_UNKNOWN SPacketType = -1
	SP_CONNECT SPacketType = iota
	SP_DISCONNECT
	SP_EVENT
	SP_ACK
	SP_CONNECT_ERROR
	SP_BINARY_EVENT
	SP_BINARY_ACK
)

func (t SPacketType) String() string {
	switch t {
	case SP_CONNECT:
		return "CONNECT"
	case SP_DISCONNECT:
		return "DISCONNECT"
	case SP_EVENT:
		return "EVENT"
	case SP_ACK:
		return "ACK"
	case SP_CONNECT_ERROR:
		return "CONNECT_ERROR"
	case SP_BINARY_EVENT:
		return "BINARY_EVENT"
	case SP_BINARY_ACK:
		return "BINARY_ACK"
	}
	return "UNKNOWN"
}

func (t SPacketType) ID() string {
	switch t {
	case SP_CONNECT:
		return "0"
	case SP_DISCONNECT:
		return "1"
	case SP_EVENT:
		return "2"
	case SP_ACK:
		return "3"
	case SP_CONNECT_ERROR:
		return "4"
	case SP_BINARY_EVENT:
		return "5"
	case SP_BINARY_ACK:
		return "6"
	}
	return "-"
}

func ParseSPT(id string) SPacketType {
	if len(id) != 1 {
		return SP_UNKNOWN
	}
	switch id {
	case "0":
		return SP_CONNECT
	case "1":
		return SP_DISCONNECT
	case "2":
		return SP_EVENT
	case "3":
		return SP_ACK
	case "4":
		return SP_CONNECT_ERROR
	case "5":
		return SP_BINARY_EVENT
	case "6":
		return SP_BINARY_ACK
	}
	return SP_UNKNOWN
}

func GetSPT(id byte) SPacketType {
	switch id {
	case 0:
		return SP_CONNECT
	case 1:
		return SP_DISCONNECT
	case 2:
		return SP_EVENT
	case 3:
		return SP_ACK
	case 4:
		return SP_CONNECT_ERROR
	case 5:
		return SP_BINARY_EVENT
	case 6:
		return SP_BINARY_ACK
	}
	return SP_UNKNOWN
}

type EPacket struct {
	typ  EPacketType
	data []byte
}

func (p *EPacket) String() string {
	return p.typ.ID() + (string)(p.data)
}

func (p *EPacket) Bytes() []byte {
	if p.data == nil {
		return ([]byte)(p.typ.ID())
	}
	buf := bytes.NewBuffer(nil)
	buf.Grow(1 + len(p.data))
	buf.WriteString(p.typ.ID())
	buf.Write(p.data)
	return buf.Bytes()
}

func (p *EPacket) WriteTo(w io.Writer) (n int, err error) {
	return w.Write(p.Bytes())
}

func (p *EPacket) ReadFrom(r io.Reader) (n int, err error) {
	t := []byte{0}
	n, err = r.Read(t)
	if err != nil || n == 0 {
		return
	}
	p.typ = GetEPT(t[0] - '0')
	if p.typ == EP_UNKNOWN {
		return n, fmt.Errorf("Unexpected packet id %c", t[0])
	}
	p.data, err = io.ReadAll(r)
	if err != nil {
		return
	}
	n += len(p.data)
	return
}

type SPacket struct {
	typ  SPacketType
	nsp  string
	id   uint64
	data any
	// attachments
}

func (p *SPacket) Bytes() []byte {
	var dbuf []byte
	if p.data != nil {
		dbuf = json.EncodeJson(p.data)
	}
	ids := strconv.FormatUint(p.id, 10)
	buf := bytes.NewBuffer(nil)
	buf.Grow(3 + len(p.nsp) + len(ids) + len(dbuf)) // (1 + 1 + len(p.nsp) + 1 + len(ids)? + len(dbuf))
	buf.WriteString(p.typ.ID())
	if len(p.nsp) > 0 {
		buf.WriteString("/" + p.nsp + ",")
	}
	if p.id > 0 {
		buf.WriteString(ids)
	}
	if len(dbuf) > 0 {
		buf.Write(dbuf)
	}
	return buf.Bytes()
}

func (p *SPacket) WriteTo(w io.Writer) (n int, err error) {
	return w.Write(p.Bytes())
}

func (p *SPacket) ReadFrom(r io.Reader) (n int, err error) {
	var b []byte
	b, err = io.ReadAll(r)
	if err != nil {
		return
	}
	n = len(b)
	err = p.ParseBuffer(bytes.NewReader(b))
	return
}

func (p *SPacket) ParseBuffer(r *bytes.Reader) (err error) {
	var (
		b   byte
		s0  int64
		s   int64
		buf []byte
	)
	// parse type
	b, err = r.ReadByte()
	if err != nil {
		return
	}
	p.typ = GetSPT(b - '0')
	if p.typ == SP_UNKNOWN {
		return fmt.Errorf("Unexpected packet id %c", b)
	}

	// parse namespace
	b, err = r.ReadByte()
	if err != nil {
		return
	}
	if b == '/' {
		s, err = r.Seek(0, io.SeekCurrent)
		if err != nil {
			return
		}
		for {
			b, err = r.ReadByte()
			if err != nil {
				return
			}
			if b == ',' {
				break
			}
		}
		s0, err = r.Seek(0, io.SeekCurrent)
		if err != nil {
			return
		}
		buf = make([]byte, s0-s-1)
		_, err = r.ReadAt(buf, s+1)
		if err != nil {
			return
		}
		p.nsp = (string)(buf)
		b, err = r.ReadByte()
		if err != nil {
			return
		}
		s0++
	} else {
		s0, err = r.Seek(0, io.SeekCurrent)
		if err != nil {
			return
		}
	}
	// parse id
	for '0' <= b && b <= '9' {
		b, err = r.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			return
		}
	}
	r.UnreadByte()
	s, err = r.Seek(0, io.SeekCurrent)
	if err != nil {
		return
	}
	if s+1 > s0 {
		buf = make([]byte, s-s0+1)
		_, err = r.ReadAt(buf, s0-1)
		if err != nil {
			return
		}
		p.id, err = strconv.ParseUint((string)(buf), 10, 64)
		if err != nil {
			return
		}
	}
	// parse data
	buf = make([]byte, r.Len())
	_, err = r.Read(buf)
	if err != nil {
		return
	}
	err = json.DecodeJson(buf, &p.data)
	return
}

var WsDialer *websocket.Dialer = &websocket.Dialer{
	Proxy:            http.ProxyFromEnvironment,
	HandshakeTimeout: 45 * time.Second,
	TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, // Skip verify because the author was lazy
	},
}

type ESocket struct {
	mux sync.Mutex

	connected  bool
	connecting bool
	connsign   chan struct{}
	msgbuf     []*EPacket
	url        string
	header     http.Header
	sid        string

	ConnectHandle    func(s *ESocket)
	DisconnectHandle func(s *ESocket)
	ErrorHandle      func(s *ESocket, err error)
	PongHandle       func(s *ESocket, data []byte)
	MessageHandle    func(s *ESocket, data []byte)

	Dialer *websocket.Dialer
	wsconn *websocket.Conn
}

func NewESocket(_d ...*websocket.Dialer) *ESocket {
	d := *WsDialer
	if len(_d) > 0 {
		d = *_d[0]
	}
	return &ESocket{
		connected:  false,
		connecting: false,
		connsign:   make(chan struct{}, 0),
		msgbuf:     make([]*EPacket, 0),
		sid:        "",

		Dialer: &d,
		wsconn: nil,
	}
}

func (s *ESocket) Dial(url string, _h ...http.Header) (err error) {
	if s.wsconn != nil {
		panic("s.wsconn != nil")
	}
	s.url = url
	s.header = http.Header{}
	if len(_h) > 0 {
		s.header = _h[0]
	}
	var wsconn *websocket.Conn
	wsconn, _, err = s.Dialer.Dial(s.url, s.header)
	if err != nil {
		return
	}
	s.wsconn = wsconn
	s.connected = true
	s.connecting = true

	oldclose := s.wsconn.CloseHandler()
	s.wsconn.SetCloseHandler(func(code int, text string) (err error) {
		err = oldclose(code, text)
		s.wsCloseHandler(code, text)
		return
	})
	go s._reader()
	return nil
}

func (s *ESocket) wsCloseHandler(code int, text string) (err error) {
	s.connecting = false
	if code == websocket.CloseNormalClosure {
		logWarn("Websocket disconnected")
	} else {
		err := fmt.Errorf("Websocket disconnected(%d): %s", code, text)
		logError(err)
		// s.Reconnect()
		if s.ErrorHandle != nil {
			s.ErrorHandle(s, err)
		}
	}
	return
}

func (s *ESocket) Reconnect() (err error) {
	if s.connecting {
		return nil
	}
	if s.wsconn == nil {
		return nil
	}
	var wsconn *websocket.Conn
	wsconn, _, err = s.Dialer.Dial(s.url, s.header)
	if err != nil {
		return
	}
	s.wsconn = wsconn
	s.connecting = true
	oldclose := s.wsconn.CloseHandler()
	s.wsconn.SetCloseHandler(func(code int, text string) (err error) {
		err = oldclose(code, text)
		s.wsCloseHandler(code, text)
		return
	})
	mb := s.msgbuf
	s.msgbuf = make([]*EPacket, 0)
	for _, p := range mb {
		s.Emit(p)
	}
	select {
	case <-s.connsign:
	default:
	}
	return nil
}

func (s *ESocket) Close() (err error) {
	if !s.connecting {
		return nil
	}
	s.connecting = false
	err = s.wsconn.Close()
	s.wsconn = nil
	select {
	case <-s.connsign:
	default:
	}
	return
}

func (s *ESocket) IsConn() bool {
	return s.connecting
}

func (s *ESocket) _reader() {
	var (
		code int
		r    io.Reader
		err  error
		pkt  *EPacket
	)
	var (
		obj json.JsonObj
	)
	for s.wsconn != nil {
		if !s.connecting {
			logDebug("Waiting for reconnect")
			s.connsign <- struct{}{}
			if s.wsconn == nil {
				return
			}
			logDebug("Reconnect successed")
		}
		logDebug("reading message")
		code, r, err = s.wsconn.NextReader()
		if err != nil {
			logError("Error when try read websocket:", err)
			s.wsconn.Close()
			if s.ErrorHandle != nil {
				s.ErrorHandle(s, err)
			}
			return
		}
		if code != websocket.TextMessage {
			continue
		}
		pkt = &EPacket{}
		_, err = pkt.ReadFrom(r)
		if err != nil {
			logError("Error when parsing packet:", err)
			continue
		}
		logDebug("Engine.io: recv packet:", string(pkt.Bytes()))
		switch pkt.typ {
		case EP_OPEN:
			err = json.DecodeJson(pkt.data, &obj)
			if err == nil {
				s.sid = obj.GetString("sid")
				logDebug("Engine.io: connected id:", s.sid)
				if s.ConnectHandle != nil {
					s.ConnectHandle(s)
				}
			}
		case EP_CLOSE:
			logDebug("disconnecting", s)
			if s.wsconn != nil {
				if s.DisconnectHandle != nil {
					s.DisconnectHandle(s)
				}
				s.Close()
			}
			return
		case EP_PING:
			s.Emit(&EPacket{typ: EP_PONG, data: pkt.data})
		case EP_PONG:
			if s.PongHandle != nil {
				s.PongHandle(s, pkt.data)
			}
		case EP_MESSAGE:
			if s.MessageHandle != nil {
				s.MessageHandle(s, pkt.data)
			}
		default:
			logError("Unknown engine.io packet:", pkt.typ)
		}
		if err != nil {
			logError("Error when decode message:", err)
		}
	}
}

func (s *ESocket) Emit(p *EPacket) (err error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	if !s.connecting {
		s.msgbuf = append(s.msgbuf, p)
		return
	}
	var w io.WriteCloser
	w, err = s.wsconn.NextWriter(websocket.TextMessage)
	if err != nil {
		return
	}
	_, err = p.WriteTo(w)
	if err != nil {
		w.Close()
		return
	}
	logDebug("Send message:", string(p.Bytes()))
	return w.Close()
}

type Socket struct {
	connecting bool
	ctx        context.Context
	cancel     context.CancelCauseFunc

	ackid   atomic.Uint64
	ackMux  sync.Mutex
	ackcall map[uint64]chan json.JsonArr

	ConnectHandle    func(s *Socket)
	DisconnectHandle func(s *Socket)
	ErrorHandle      func(s *Socket)
	handles          map[string]func(string, json.JsonArr)
	sid              string

	io *ESocket
}

func NewSocket(ctx context.Context, io *ESocket) (s *Socket) {
	s = &Socket{
		connecting: false,

		ackcall: make(map[uint64]chan json.JsonArr),

		handles: make(map[string]func(string, json.JsonArr)),
		sid:     "",

		io: io,
	}
	s.ctx, s.cancel = context.WithCancelCause(ctx)
	s.io.ConnectHandle = func(*ESocket) {
		s.send(&SPacket{typ: SP_CONNECT})
	}
	s.io.DisconnectHandle = func(*ESocket) {
		s.Close()
	}
	s.io.ErrorHandle = func(_ *ESocket, err error) {
		s.cancel(err)
		if s.ErrorHandle != nil {
			s.ErrorHandle(s)
		}
	}
	s.io.MessageHandle = func(_ *ESocket, data []byte) {
		var (
			err error
			obj json.JsonObj
			arr json.JsonArr
		)
		pkt := &SPacket{}
		err = pkt.ParseBuffer(bytes.NewReader(data))
		if err != nil {
			logError("Error when parsing sio packet:", err)
			return
		}
		err = nil
		switch pkt.typ {
		case SP_CONNECT:
			if d, ok := pkt.data.(map[string]any); ok {
				obj = (json.JsonObj)(d)
				s.sid = obj.GetString("sid")
				logDebug("Socket.io: connected id:", s.sid)
				s.connecting = true
				if s.ConnectHandle != nil {
					s.ConnectHandle(s)
				}
			}
		case SP_DISCONNECT:
			s.DisconnectHandle(s)
		case SP_EVENT:
			arr = (json.JsonArr)(pkt.data.([]any))
			e := arr.GetString(0)
			if h, ok := s.handles[e]; ok {
				h(e, arr[1:])
			}
		case SP_ACK:
			s.ackMux.Lock()
			defer s.ackMux.Unlock()
			logDebug("ackcall:", pkt.id, s.ackcall)
			if h, ok := s.ackcall[pkt.id]; ok {
				delete(s.ackcall, pkt.id)
				arr = (json.JsonArr)(pkt.data.([]any))
				h <- arr
			}
		case SP_CONNECT_ERROR:
			logError("Socket.io: connect error:", pkt.data)
			panic(pkt.data)
		// case SP_BINARY_EVENT:
		// 	// TODO
		// case SP_BINARY_ACK:
		// 	// TODO
		default:
			logError("Unsupported sio packet type:", pkt.typ.String())
		}
		if err != nil {
			logError("Error when decode message:", err)
		}
	}
	if s.io.IsConn() {
		s.io.ConnectHandle(s.io)
	}
	return
}

func (s *Socket) Close() (err error) {
	if !s.connecting {
		return nil
	}
	if s.DisconnectHandle != nil {
		s.DisconnectHandle(s)
	}
	s.send(&SPacket{typ: SP_DISCONNECT})
	s.connecting = false
	return nil
}

func (s *Socket) IsConn() bool {
	return s.connecting
}

func (s *Socket) GetIO() *ESocket {
	return s.io
}

func (s *Socket) On(event string, call func(string, json.JsonArr)) *Socket {
	s.handles[event] = call
	return s
}

func (s *Socket) send(p *SPacket) (err error) {
	return s.io.Emit(&EPacket{typ: EP_MESSAGE, data: p.Bytes()})
}

func (s *Socket) Emit(event string, objs ...any) (err error) {
	return s.send(&SPacket{typ: SP_EVENT, data: (json.JsonArr)(append([]any{event}, objs...))})
}

func (s *Socket) EmitAck(event string, objs ...any) (res json.JsonArr, err error) {
	id := s.ackid.Add(1)
	err = s.send(&SPacket{typ: SP_EVENT, id: id, data: (json.JsonArr)(append([]any{event}, objs...))})
	if err != nil {
		return
	}
	resCh := make(chan json.JsonArr, 1)

	s.ackMux.Lock()
	s.ackcall[id] = resCh
	s.ackMux.Unlock()
	select {
	case ret := <-resCh:
		res = ret.GetArray(0)
	case <-s.ctx.Done():
		err = context.Cause(s.ctx)
	}
	return
}
