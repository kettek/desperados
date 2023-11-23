/*
This file contains the implementation of the multicast protocol functions.
*/
package dnet

import (
	"net"
	"strings"
	"time"

	"golang.org/x/net/ipv4"
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

func (m *Multicaster) RecvAddr() *net.UDPAddr {
	return m.conn.LocalAddr().(*net.UDPAddr)
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

func NewMulticaster(address, sendAddress string) (*Multicaster, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	// Acquire the primary interface address if none is specified.
	if sendAddress == "" {
		c, err := net.Dial("udp", "8.8.8.8:80")
		if err != nil {
			return nil, err
		}
		defer c.Close()
		sendAddress = c.LocalAddr().String()
	}

	// Allow lazy address specification.
	if !strings.Contains(sendAddress, ":") {
		sendAddress += ":0"
	}

	sendAddr, err := net.ResolveUDPAddr("udp", sendAddress)
	if err != nil {
		return nil, err
	}
	// Ensure that the port is 0.
	sendAddr.Port = 0

	var miface net.Interface
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.HardwareAddr == nil {
			continue
		}
		if addrs, err := iface.Addrs(); err == nil {
			for _, a := range addrs {
				// FIXME: Should probably do a type assertion first.
				if a.(*net.IPNet).IP.Equal(sendAddr.IP) {
					miface = iface
					break
				}
			}
		}
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	pc := ipv4.NewPacketConn(conn)
	if err := pc.JoinGroup(&miface, addr); err != nil {
		return nil, err
	}

	if loop, err := pc.MulticastLoopback(); err == nil {
		if !loop {
			if err := pc.SetMulticastLoopback(true); err != nil {
				return nil, err
			}
		}
	}

	// Set up the send connection.
	sendConn, err := net.DialUDP("udp", sendAddr, addr)
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
				m.conn.Close()
				m.sendConn.Close()
				m.conn = nil
				m.sendConn = nil
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
			if n == 9 {
				if string(data[4:9]) == "!ping" {
					// Send a pong.
					conn.WriteToUDP(append([]byte("DESP"), []byte("!pong")...), raddr)
					continue
				} else if string(data[4:9]) == "!pong" {
					continue
				}
			}
			m.recv <- &MulticastMessage{raddr, data[4:n]}
			conn.SetReadDeadline(time.Now().Add(time.Second * 1))
		}
	}()

	return m, nil
}
