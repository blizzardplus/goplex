package pt

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"time"
	"golang.org/x/net/proxy"
	"net/url"
	"strconv"
	"encoding/binary"
)

const (
	socksVersion         = 0x04
	socksCmdConnect      = 0x01
	socksResponseVersion = 0x00
	socksRequestGranted  = 0x5a
	socksRequestRejected = 0x5b
)

// Put a sanity timeout on how long we wait for a SOCKS request.
const socksRequestTimeout = 5 * time.Second

// SocksRequest describes a SOCKS request.
type SocksRequest struct {
	// The endpoint requested by the client as a "host:port" string.
	Target string
	// The userid string sent by the client.
	Username string
	// The parsed contents of Username as a key–value mapping.
	Args Args
}

// SocksConn encapsulates a net.Conn and information associated with a SOCKS request.
type SocksConn struct {
	net.Conn
	Req SocksRequest
}

// Send a message to the proxy client that access to the given address is
// granted. If the IP field inside addr is not an IPv4 address, the IP portion
// of the response will be four zero bytes.
func (conn *SocksConn) Grant(addr *net.TCPAddr) error {
	return sendSocks4aResponseGranted(conn, addr)
}

// Send a message to the proxy client that access was rejected or failed.
func (conn *SocksConn) Reject() error {
	return sendSocks4aResponseRejected(conn)
}

// SocksListener wraps a net.Listener in order to read a SOCKS request on Accept.
//
// 	func handleConn(conn *pt.SocksConn) error {
// 		defer conn.Close()
// 		remote, err := net.Dial("tcp", conn.Req.Target)
// 		if err != nil {
// 			conn.Reject()
// 			return err
// 		}
// 		defer remote.Close()
// 		err = conn.Grant(remote.RemoteAddr().(*net.TCPAddr))
// 		if err != nil {
// 			return err
// 		}
// 		// do something with conn and remote
// 		return nil
// 	}
// 	...
// 	ln, err := pt.ListenSocks("tcp", "127.0.0.1:0")
// 	if err != nil {
// 		panic(err.Error())
// 	}
// 	for {
// 		conn, err := ln.AcceptSocks()
// 		if err != nil {
// 			log.Printf("accept error: %s", err)
// 			if e, ok := err.(net.Error); ok && e.Temporary() {
// 				continue
// 			}
// 			break
// 		}
// 		go handleConn(conn)
// 	}
type SocksListener struct {
	net.Listener
}

// Open a net.Listener according to network and laddr, and return it as a
// SocksListener.
func ListenSocks(network, laddr string) (*SocksListener, error) {
	ln, err := net.Listen(network, laddr)
	if err != nil {
		return nil, err
	}
	return NewSocksListener(ln), nil
}

// Create a new SocksListener wrapping the given net.Listener.
func NewSocksListener(ln net.Listener) *SocksListener {
	return &SocksListener{ln}
}

// Accept is the same as AcceptSocks, except that it returns a generic net.Conn.
// It is present for the sake of satisfying the net.Listener interface.
func (ln *SocksListener) Accept() (net.Conn, error) {
	return ln.AcceptSocks()
}

// Call Accept on the wrapped net.Listener, do SOCKS negotiation, and return a
// SocksConn. After accepting, you must call either conn.Grant or conn.Reject
// (presumably after trying to connect to conn.Req.Target).
//
// Errors returned by AcceptSocks may be temporary (for example, EOF while
// reading the request, or a badly formatted userid string), or permanent (e.g.,
// the underlying socket is closed). You can determine whether an error is
// temporary and take appropriate action with a type conversion to net.Error.
// For example:
//
// 	for {
// 		conn, err := ln.AcceptSocks()
// 		if err != nil {
// 			if e, ok := err.(net.Error); ok && e.Temporary() {
// 				log.Printf("temporary accept error; trying again: %s", err)
// 				continue
// 			}
// 			log.Printf("permanent accept error; giving up: %s", err)
// 			break
// 		}
// 		go handleConn(conn)
// 	}
func (ln *SocksListener) AcceptSocks() (*SocksConn, error) {
retry:
	c, err := ln.Listener.Accept()
	if err != nil {
		return nil, err
	}
	conn := new(SocksConn)
	conn.Conn = c
	err = conn.SetDeadline(time.Now().Add(socksRequestTimeout))
	if err != nil {
		conn.Close()
		goto retry
	}
	conn.Req, err = readSocks4aConnect(conn)
	if err != nil {
		conn.Close()
		goto retry
	}
	err = conn.SetDeadline(time.Time{})
	if err != nil {
		conn.Close()
		goto retry
	}
	return conn, nil
}

// Returns "socks4", suitable to be included in a call to Cmethod.
func (ln *SocksListener) Version() string {
	return "socks4"
}

// Read a SOCKS4a connect request. Returns a SocksRequest.
func readSocks4aConnect(s io.Reader) (req SocksRequest, err error) {
	r := bufio.NewReader(s)

	var h [8]byte
	_, err = io.ReadFull(r, h[:])
	if err != nil {
		return
	}
	if h[0] != socksVersion {
		err = fmt.Errorf("SOCKS header had version 0x%02x, not 0x%02x", h[0], socksVersion)
		return
	}
	if h[1] != socksCmdConnect {
		err = fmt.Errorf("SOCKS header had command 0x%02x, not 0x%02x", h[1], socksCmdConnect)
		return
	}

	var usernameBytes []byte
	usernameBytes, err = r.ReadBytes('\x00')
	if err != nil {
		return
	}
	req.Username = string(usernameBytes[:len(usernameBytes)-1])

	req.Args, err = parseClientParameters(req.Username)
	if err != nil {
		return
	}

	var port int
	var host string

	port = int(h[2])<<8 | int(h[3])<<0
	if h[4] == 0 && h[5] == 0 && h[6] == 0 && h[7] != 0 {
		var hostBytes []byte
		hostBytes, err = r.ReadBytes('\x00')
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		host = string(hostBytes[:len(hostBytes)-1])
	} else {
		host = net.IPv4(h[4], h[5], h[6], h[7]).String()
	}

	if r.Buffered() != 0 {
		err = fmt.Errorf("%d bytes left after SOCKS header", r.Buffered())
		return
	}

	req.Target = fmt.Sprintf("%s:%d", host, port)
	return
}

// Send a SOCKS4a response with the given code and address. If the IP field
// inside addr is not an IPv4 address, the IP portion of the response will be
// four zero bytes.
func sendSocks4aResponse(w io.Writer, code byte, addr *net.TCPAddr) error {
	var resp [8]byte
	resp[0] = socksResponseVersion
	resp[1] = code
	resp[2] = byte((addr.Port >> 8) & 0xff)
	resp[3] = byte((addr.Port >> 0) & 0xff)
	ipv4 := addr.IP.To4()
	if ipv4 != nil {
		resp[4] = ipv4[0]
		resp[5] = ipv4[1]
		resp[6] = ipv4[2]
		resp[7] = ipv4[3]
	}
	_, err := w.Write(resp[:])
	return err
}

var emptyAddr = net.TCPAddr{IP: net.IPv4(0, 0, 0, 0), Port: 0}

// Send a SOCKS4a response code 0x5a.
func sendSocks4aResponseGranted(w io.Writer, addr *net.TCPAddr) error {
	return sendSocks4aResponse(w, socksRequestGranted, addr)
}

// Send a SOCKS4a response code 0x5b (with an all-zero address).
func sendSocks4aResponseRejected(w io.Writer) error {
	return sendSocks4aResponse(w, socksRequestRejected, &emptyAddr)
}



//From https://github.com/Bogdan-D/go-socks4

func SocksInit() {
	proxy.RegisterDialerType("socks4", func(u *url.URL, d proxy.Dialer) (proxy.Dialer, error) {
		return &socks4{url: u, dialer: d}, nil
	})
}

const (
	socks_version = 0x04
	socks_connect = 0x01
	socks_bind    = 0x02

	socks_ident = "nobody@0.0.0.0"

	access_granted         = 0x5a
	access_rejected        = 0x5b
	access_identd_required = 0x5c
	access_identd_failed   = 0x5d

	ErrWrongURL      = "wrong server url"
	ErrWrongConnType = "no support for connections of type"
	ErrConnFailed    = "connection failed to socks4 server"
	ErrHostUnknown   = "unable to find IP address of host"
	ErrSocksServer   = "socks4 server error"
	ErrConnRejected  = "connection rejected"
	ErrIdentRequired = "socks4 server require valid identd"
)

type socks4 struct {
	url    *url.URL
	dialer proxy.Dialer
}
type socks4Error struct {
	message string
	details interface{}
}

func (s *socks4Error) String() string {
	return s.message
}

func (s *socks4Error) Error() string {
	if s.details == nil {
		return s.message
	}

	return fmt.Sprintf("%s: %v", s.message, s.details)
}

func Dial(network, address string) (net.Conn, error) {
	var socksProx *socks4
	return socksProx.Dial(network, address)
}

func (s *socks4) Dial(network, addr string) (c net.Conn, err error) {
	var buf []byte

	switch network {
	case "tcp", "tcp4":
	default:
		return nil, &socks4Error{message: ErrWrongConnType, details: network}
	}

	c, err = s.dialer.Dial(network, s.url.Host)
	if err != nil {
		return nil, &socks4Error{message: ErrConnFailed, details: err}
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, &socks4Error{message: ErrWrongURL, details: err}
	}

	//
	ipAddr := net.ParseIP(host)
	
	socksV4a := false
	var ip4 net.IP
	if ipAddr == nil{
		ip, err := net.ResolveIPAddr("ip4", host)
		if err != nil {
			return nil, &socks4Error{message: ErrHostUnknown, details: err}
		}
		ip4 = ip.IP.To4()
	}else {
		socksV4a = true
		ip, err := net.ResolveIPAddr("ip4", "0.0.0.255")
		if err != nil {
			return nil, &socks4Error{message: ErrHostUnknown, details: err}
		}
		ip4 = ip.IP.To4()
	}
	


	var bport [2]byte
	iport, _ := strconv.Atoi(port)
	binary.BigEndian.PutUint16(bport[:], uint16(iport))

	buf = []byte{socks_version, socks_connect}
	buf = append(buf, bport[:]...)
	buf = append(buf, ip4...)
	//buf = append(buf, socks_ident...)
	buf = append(buf, 0)
	
	if socksV4a {
		//hostLength := utf8.RuneCountInString(host)
		//hostBytes := []byte(host)	
		buf = append(buf, ipAddr[12:16]...)
	}

	i, err := c.Write(buf)
	if err != nil {
		return nil, &socks4Error{message: ErrSocksServer, details: err}
	}
	if l := len(buf); i != l {
		return nil, &socks4Error{message: ErrSocksServer, details: fmt.Sprintf("write %d bytes, expected %d", i, l)}
	}

	var resp [8]byte
	i, err = c.Read(resp[:])
	if err != nil && err != io.EOF {
		return nil, &socks4Error{message: ErrSocksServer, details: err}
	}
	if i != 8 {
		return nil, &socks4Error{message: ErrSocksServer, details: fmt.Sprintf("read %d bytes, expected 8", i)}
	}

	switch resp[1] {
	case access_granted:
		return c, nil
	case access_identd_required, access_identd_failed:
		return nil, &socks4Error{message: ErrIdentRequired, details: strconv.FormatInt(int64(resp[1]), 16)}
	default:
		c.Close()
		return nil, &socks4Error{message: ErrConnRejected, details: strconv.FormatInt(int64(resp[1]), 16)}
	}
}
