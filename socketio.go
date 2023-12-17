/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2023 Kevin Z <zyxkad@gmail.com>
 * All rights reserved
 *
 *  This program is free software: you can redistribute it and/or modify
 *  it under the terms of the GNU Affero General Public License as published
 *  by the Free Software Foundation, either version 3 of the License, or
 *  (at your option) any later version.
 *
 *  This program is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *  GNU Affero General Public License for more details.
 * 
 *  You should have received a copy of the GNU Affero General Public License
 *  along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var (
	ErrSocketConnected = errors.New("Engine.io: Socket already connected")
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
	data []byte
	// attachments
}

func (p *SPacket) Bytes() []byte {
	ids := strconv.FormatUint(p.id, 10)
	buf := bytes.NewBuffer(make([]byte, 0, 3+len(p.nsp)+len(ids)+len(p.data))) // (1 + 1 + len(p.nsp) + 1 + len(ids)? + len(dbuf))
	buf.WriteString(p.typ.ID())
	if len(p.nsp) > 0 {
		buf.WriteString("/" + p.nsp + ",")
	}
	if p.id > 0 {
		buf.WriteString(ids)
	}
	buf.Write(p.data)
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
	p.data = make([]byte, r.Len())
	_, err = r.Read(p.data)
	if err != nil {
		return
	}
	return
}

func (p *SPacket) ParseData(o_ptr any) error {
	return json.Unmarshal(p.data, o_ptr)
}

var WsDialer *websocket.Dialer = &websocket.Dialer{
	Proxy:            http.ProxyFromEnvironment,
	HandshakeTimeout: 30 * time.Second,
	TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, // Skip verify because the author was lazy
	},
}

const (
	ESocketIdle int32 = iota
	ESocketDialing
	ESocketConnected
)

const maxReconnectCount = 10

type ESocket struct {
	mux sync.Mutex

	sid    string
	status atomic.Int32
	url    string
	header http.Header

	msgbuf, msgbuf2 []*EPacket
	errCount        int

	ConnectHandle    func(s *ESocket)
	DisconnectHandle func(s *ESocket)
	ErrorHandle      func(s *ESocket, err error)
	PongHandle       func(s *ESocket, data []byte)
	MessageHandle    func(s *ESocket, data []byte)

	Dialer *websocket.Dialer
	wsconn *websocket.Conn
}

func NewESocket(_d ...*websocket.Dialer) (s *ESocket) {
	d := *WsDialer
	if len(_d) > 0 {
		d = *_d[0]
	}
	s = &ESocket{
		Dialer: &d,
	}
	return
}

func (s *ESocket) Status() int32 {
	return s.status.Load()
}

func (s *ESocket) Connected() bool {
	return s.Status() == ESocketConnected
}

type ESocketDialOptions func(s *ESocket)

func WithHeader(header http.Header) ESocketDialOptions {
	return func(s *ESocket) {
		s.header = header
	}
}

func (s *ESocket) DialContext(ctx context.Context, url string, opts ...ESocketDialOptions) (err error) {
	if !s.status.CompareAndSwap(ESocketIdle, ESocketDialing) {
		return ErrSocketConnected
	}

	s.url = url

	for _, opt := range opts {
		opt(s)
	}

	wsconn, _, err := s.Dialer.DialContext(ctx, s.url, s.header)
	if err != nil {
		s.status.Store(ESocketIdle)
		return
	}
	s.wsconn = wsconn
	s.errCount = 0

	oldclose := wsconn.CloseHandler()
	wsconn.SetCloseHandler(func(code int, text string) (err error) {
		err = oldclose(code, text)
		s.wsCloseHandler(code, text)
		return
	})

	s.status.Store(ESocketConnected)

	go s._reader()

	s.mux.Lock()
	mb := s.msgbuf
	s.msgbuf, s.msgbuf2 = s.msgbuf2[:0], s.msgbuf
	s.mux.Unlock()
	for _, p := range mb {
		s.Emit(p)
	}
	return nil
}

func (s *ESocket) wsCloseHandler(code int, text string) (err error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.status.Store(ESocketIdle)

	wer := &websocket.CloseError{code, text}
	if code == websocket.CloseNormalClosure {
		logWarn("Engine.io: Websocket disconnected")
	} else {
		logError("Engine.io: Websocket disconnected:", wer)
		if s.errCount++; s.errCount > maxReconnectCount {
			return wer
		}
		logDebug("Engine.io: Reconnecting ...")
		if err = s.DialContext(context.TODO(), s.url); err != nil {
			logError("Engine.io: Reconnect failed:", err)
			return
		}
		if s.ErrorHandle != nil {
			go s.ErrorHandle(s, wer)
		}
	}
	return
}

func (s *ESocket) Close() (err error) {
	if !s.status.CompareAndSwap(ESocketConnected, ESocketIdle) {
		return
	}
	err = s.wsconn.Close()
	s.wsconn = nil
	return
}

func (s *ESocket) _reader() {
	defer s.wsconn.Close()
	for {
		if s.Status() != ESocketConnected {
			logDebug("Engine.io: Connection", s, "broke")
			return
		}
		logDebug("Engine.io: reading message")
		code, r, err := s.wsconn.NextReader()
		if err != nil {
			logError("Engine.io: Error when getting next message:", err)
			if s.ErrorHandle != nil {
				s.ErrorHandle(s, err)
			}
			return
		}
		if code != websocket.TextMessage {
			continue
		}
		pkt := &EPacket{}
		_, err = pkt.ReadFrom(r)
		if err != nil {
			logError("Engine.io: Error when parsing packet:", err)
			continue
		}
		logDebug("Engine.io: recv packet:", (string)(pkt.Bytes()))
		switch pkt.typ {
		case EP_OPEN:
			var obj struct {
				Sid string `json:"sid"`
			}
			if err = json.Unmarshal(pkt.data, &obj); err == nil {
				s.sid = obj.Sid
				logDebug("Engine.io: connected id:", s.sid)
				if s.ConnectHandle != nil {
					s.ConnectHandle(s)
				}
			}
		case EP_CLOSE:
			logDebug("Engine.io: disconnecting", s)
			if s.wsconn != nil {
				if s.DisconnectHandle != nil {
					s.DisconnectHandle(s)
				}
				s.Close()
			}
			return
		case EP_PING:
			pkt.typ = EP_PONG
			s.Emit(pkt)
		case EP_PONG:
			if s.PongHandle != nil {
				s.PongHandle(s, pkt.data)
			}
		case EP_MESSAGE:
			if s.MessageHandle != nil {
				s.MessageHandle(s, pkt.data)
			}
		default:
			logError("Engine.io: Unknown packet type:", pkt.typ)
		}
		if err != nil {
			logError("Engine.io: Error when decode message:", err)
		}
	}
}

func (s *ESocket) Emit(p *EPacket) (err error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.Status() != ESocketConnected {
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
	logDebug("Engine.io: sent message:", (string)(p.Bytes()))
	return w.Close()
}

const (
	SSocketIdle int32 = iota
	SSocketDialing
	SSocketConnected
)

type Socket struct {
	io     *ESocket
	status atomic.Int32

	sid     string
	ackid   atomic.Uint64
	ackMux  sync.Mutex
	ackcall map[uint64]chan []any

	ConnectHandle    func(s *Socket)
	DisconnectHandle func(s *Socket)
	ErrorHandle      func(s *Socket)
	handles          map[string]func(string, []any)
}

func NewSocket(io *ESocket) (s *Socket) {
	s = &Socket{
		io:      io,
		ackcall: make(map[uint64]chan []any),
		handles: make(map[string]func(string, []any)),
	}
	s.io.MessageHandle = func(_ *ESocket, data []byte) {
		pkt := &SPacket{}
		err := pkt.ParseBuffer(bytes.NewReader(data))
		if err != nil {
			logError("Error when parsing socket.io packet:", err)
			return
		}
		switch pkt.typ {
		case SP_CONNECT:
			var obj struct {
				Sid string `json:"sid"`
			}
			if err := pkt.ParseData(&obj); err == nil {
				s.sid = obj.Sid
				logDebug("Socket.io: connected id:", s.sid)
				s.status.Store(SSocketConnected)
				if s.ConnectHandle != nil {
					s.ConnectHandle(s)
				}
			}
		case SP_DISCONNECT:
			if s.DisconnectHandle != nil {
				s.DisconnectHandle(s)
			}
		case SP_EVENT:
			var arr []any
			if err := pkt.ParseData(&arr); err == nil {
				e := arr[0].(string)
				if h, ok := s.handles[e]; ok {
					h(e, arr[1:])
				}
			}
		case SP_ACK:
			s.ackMux.Lock()
			ch, ok := s.ackcall[pkt.id]
			delete(s.ackcall, pkt.id)
			s.ackMux.Unlock()
			if ok {
				var arr []any
				pkt.ParseData(&arr)
				select {
				case ch <- arr:
				default:
					logError("Socket.io: Couldn't send ack packet through the channel")
				}
			}
		case SP_CONNECT_ERROR:
			logError("Socket.io: connect error:", pkt.data)
			if s.ErrorHandle != nil {
				s.ErrorHandle(s)
			}
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
	s.io.DisconnectHandle = func(*ESocket) {
		s.Close()
	}
	s.io.ErrorHandle = func(*ESocket, error) {
		if s.ErrorHandle != nil {
			s.ErrorHandle(s)
		}
	}
	s.io.ConnectHandle = func(*ESocket) {
		s.send(&SPacket{typ: SP_CONNECT})
	}
	if s.io.Connected() {
		s.send(&SPacket{typ: SP_CONNECT})
	}
	return
}

func (s *Socket) Close() (err error) {
	if !s.status.CompareAndSwap(SSocketConnected, SSocketIdle) {
		return nil
	}
	if s.DisconnectHandle != nil {
		s.DisconnectHandle(s)
	}
	return s.send(&SPacket{typ: SP_DISCONNECT})
}

func (s *Socket) IO() *ESocket {
	return s.io
}

func (s *Socket) On(event string, call func(string, []any)) *Socket {
	s.handles[event] = call
	return s
}

func (s *Socket) send(p *SPacket) (err error) {
	return s.io.Emit(&EPacket{typ: EP_MESSAGE, data: p.Bytes()})
}

func (s *Socket) Emit(event string, objs ...any) (err error) {
	pkt := &SPacket{typ: SP_EVENT}
	pkt.data, err = json.Marshal(append([]any{event}, objs...))
	if err != nil {
		return
	}
	return s.send(pkt)
}

func (s *Socket) EmitAckContext(ctx context.Context, event string, objs ...any) (res []any, err error) {
	id := s.ackid.Add(1)
	pkt := &SPacket{typ: SP_EVENT, id: id}
	pkt.data, err = json.Marshal(append([]any{event}, objs...))
	if err != nil {
		return
	}
	resCh := make(chan []any, 1)
	go func() {
		if err = s.send(pkt); err != nil {
			return
		}
		if err = ctx.Err(); err != nil {
			return
		}
		s.ackMux.Lock()
		s.ackcall[id] = resCh
		s.ackMux.Unlock()
	}()
	select {
	case ret := <-resCh:
		res = ret[0].([]any)
	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}

func (s *Socket) EmitAck(event string, objs ...any) (res []any, err error) {
	return s.EmitAckContext(context.Background(), event, objs...)
}
