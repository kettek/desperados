/*
This file contains the implementation of the multicast protocol functions.
*/
package dnet

import (
	"net"
	"time"
)

type MulticastMessage struct {
	Addr *net.UDPAddr
	Data []byte
}

type Multicaster struct {
	conn     *net.UDPConn
	sendConn *net.UDPConn
	recv     chan *MulticastMessage
	close    chan struct{}
}

func (m *Multicaster) SendAddr() *net.UDPAddr {
	return m.sendConn.LocalAddr().(*net.UDPAddr)
}

func (m *Multicaster) Closed() bool {
	return m.conn == nil
}

func (m *Multicaster) SetRecv(recv chan *MulticastMessage) {
	m.recv = recv
}

func (m *Multicaster) Recv() chan *MulticastMessage {
	return m.recv
}

func (m *Multicaster) Send(data []byte) {
	m.sendConn.Write(append([]byte("DESP"), data...))
}

func (m *Multicaster) Close() {
	m.close <- struct{}{}
}

func NewMulticaster(address string) (*Multicaster, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}

	// Set up the send connection.
	sendConn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}

	conn.SetReadBuffer(8192)
	conn.SetReadDeadline(time.Now().Add(time.Second * 1))

	m := &Multicaster{
		recv:     make(chan *MulticastMessage),
		close:    make(chan struct{}),
		sendConn: sendConn,
		conn:     conn,
	}

	go func() {
		for {
			// Check if we should close.
			select {
			case <-m.close:
				m.conn = nil
				return
			default:
			}

			data := make([]byte, 8192)
			n, raddr, err := conn.ReadFromUDP(data)
			if err != nil {
				if e, ok := err.(net.Error); ok && e.Timeout() {
					conn.SetReadDeadline(time.Now().Add(time.Second * 1))
					continue
				}
				panic(err)
			}
			if n < 4 || string(data[:4]) != "DESP" {
				continue
			}
			m.recv <- &MulticastMessage{raddr, data[4:n]}
			conn.SetReadDeadline(time.Now().Add(time.Second * 1))
		}
	}()

	return m, nil
}
