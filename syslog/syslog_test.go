package syslog

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"testing"
	"time"
)

const clienthost = "clienthost"

func panicf(s string, i ...interface{}) { panic(fmt.Sprintf(s, i)) }

type testServer struct {
	Addr     string
	Close    chan bool
	Messages chan string
}

func newTestServer(network string) *testServer {
	server := testServer{
		Close:    make(chan bool, 1),
		Messages: make(chan string, 20),
	}
	switch network {
	case "tcp":
		ln := server.listenTCP()
		go server.serveTCP(ln)
	case "udp":
		conn := server.listenUDP()
		go server.serveUDP(conn)
	}
	return &server
}

func (s *testServer) listenTCP() net.Listener {
	addr := s.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panicf("listen error %v", err)
	}
	if s.Addr == "" {
		s.Addr = ln.Addr().String()
	}
	return ln
}

func (s *testServer) serveTCP(ln net.Listener) {
	for {
		select {
		case <-s.Close:
			ln.Close()
			return
		default:
			conn, err := ln.Accept()
			if err != nil {
				panicf("Accept error: %v", err)
			}
			go handle(conn, s.Messages)
		}
	}
}

func (s *testServer) listenUDP() *net.UDPConn {
	addr := s.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		panicf("unexpected error %v", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		panicf("listen error %v", err)
	}
	if s.Addr == "" {
		s.Addr = conn.LocalAddr().String()
	}
	return conn
}

func (s *testServer) serveUDP(conn *net.UDPConn) {
	for {
		handle(conn, s.Messages)
		conn = s.listenUDP()
	}
}

func handle(conn io.ReadCloser, messages chan string) {
	for {
		fmt.Println("handle")
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			panicf("Read error")
		} else {
			fmt.Println("handle", string(buf[0:n]))
			messages <- string(buf[0:n])
		}
		// todo: make configurable
		if 0 == (rand.Int() % 2) {
			fmt.Println("closing")
			conn.Close()
			return
		}
	}
}

func generatePackets() []Packet {
	packets := make([]Packet, 10)
	for i, _ := range packets {
		t, _ := time.Parse(time.RFC3339, "2006-01-02T15:04:05Z07:00")
		packets[i] = Packet{
			Severity: SevInfo,
			Facility: LogLocal1,
			Time:     t,
			Hostname: clienthost,
			Tag:      "test",
			Message:  fmt.Sprintf("message %d", i),
		}
	}
	return packets
}

func TestSyslog(t *testing.T) {
	for _, network := range []string{"tcp", "udp"} {
		s := newTestServer(network)

		logger, err := Dial(clienthost, network, s.Addr, nil)
		if err != nil {
			t.Errorf("unexpected dial error %v", err)
		}
		packets := generatePackets()
		for _, p := range packets {
			logger.writePacket(p)
			time.Sleep(100 * time.Millisecond)
		}
		s.Close <- true

		for _, p := range packets {
			expected := p.Generate(0)
			if network == "tcp" {
				expected = expected + "\n"
			}
			select {
			case got := <-s.Messages:
				if got != expected {
					t.Errorf("expected %s, got %s", expected, got)
				}
			default:
				t.Errorf("expected %s, got nothing", expected)
				break
			}
		}
		if l := len(s.Messages); l != 0 {
			t.Errorf("found %d extra messages", l)
		}
	}
}
