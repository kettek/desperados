/*
This file contains tools for pinging a range of IP addresses with a particular subnet.
*/
package dnet

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

type RangerV4 struct {
	cIndex       uint8 // index of the class C position.
	IP           net.IP
	WaitDuration time.Duration
	closeChan    chan struct{}
	resultChan   chan RangerResult
}

func NewRanger(ip net.IP) *RangerV4 {
	return &RangerV4{
		IP:           ip,
		WaitDuration: 100 * time.Millisecond,
		resultChan:   make(chan RangerResult, 2),
		cIndex:       1,
	}
}

type RangerResult interface {
	Type() string
}

type RangerPong struct {
	Addr  net.Addr
	IsTCP bool
}

func (r RangerPong) Type() string {
	return "pong"
}

type RangerStep struct {
	Index uint8
	IsTCP bool
}

func (r RangerStep) Type() string {
	return "step"
}

type RangerDone struct{}

func (r RangerDone) Type() string {
	return "done"
}

var ErrRangerDone = errors.New("ranging complete")

func (r *RangerV4) SetResults(results chan RangerResult) {
	r.resultChan = results
}

func (r *RangerV4) Done() bool {
	return r.cIndex == 255
}

func (r *RangerV4) Close() {
	go func() {
		r.closeChan <- struct{}{}
	}()
}

func (r *RangerV4) Start(port int) {
	addr := r.IP.String()
	parts := strings.Split(addr, ".")

	go func() {
		for {
			select {
			case <-r.closeChan:
				r.resultChan <- RangerDone{}
				return
			default:
			}

			addr = strings.Join(parts[:3], ".") + "." + fmt.Sprintf("%d", r.cIndex) + ":" + fmt.Sprint(port)

			// Dial addr with UDP.
			if udpAddr, err := net.ResolveUDPAddr("udp", addr); err == nil {
				if udpConn, err := net.DialUDP("udp", nil, udpAddr); err == nil {
					defer udpConn.Close()
					udpConn.SetWriteDeadline(time.Now().Add(r.WaitDuration))
					if _, err := udpConn.Write(append([]byte("DESP"), []byte("!ping")...)); err == nil {
						udpConn.SetReadDeadline(time.Now().Add(r.WaitDuration))
						bytes := make([]byte, 9)
						if n, err := udpConn.Read(bytes); err == nil && n == 9 && string(bytes) == "DESP!pong" {
							// Successfully read!
							r.resultChan <- RangerPong{
								Addr: udpConn.RemoteAddr(),
							}
						}
					} else {
						if e, ok := err.(net.Error); !ok || e.Timeout() {
							// Not a timeout.
						}
						// Is a timeout.
					}
				} else {
					// no dial?
					fmt.Println(err)
				}
			} else {
				// no resolve?
				fmt.Println(err)
			}

			// Dial addr with TCP.
			/*if tcpAddr, err := net.ResolveTCPAddr("tcp", addr); err == nil {
				if tcpConn, err := net.DialTCP("tcp", nil, tcpAddr); err == nil {
					tcpConn.SetWriteDeadline(time.Now().Add(r.WaitDuration))
					if _, err := tcpConn.Write(append([]byte("DESP"), []byte("!ping")...)); err == nil {
						tcpConn.SetReadDeadline(time.Now().Add(r.WaitDuration))
						bytes := make([]byte, 9)
						if n, err := tcpConn.Read(bytes); err == nil && n == 9 && string(bytes) == "DESP!pong" {
							// Successfully read!
							r.resultChan <- &RangerPong{
								Addr:  tcpConn.RemoteAddr(),
								IsTCP: true,
							}
						}
					} else {
						if e, ok := err.(net.Error); !ok || e.Timeout() {
							// Not a timeout.
						}
						// Is a timeout.
					}
				} else {
					// no dial?
				}
			} else {
				// no resolve?
			}*/

			if r.cIndex == 255 {
				r.resultChan <- RangerDone{}
				return
			}
			r.resultChan <- RangerStep{
				IsTCP: false,
				Index: r.cIndex,
			}
			r.cIndex++
		}
	}()
}
