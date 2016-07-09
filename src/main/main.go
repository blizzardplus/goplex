package main 

	import (
	"pt"
	"net"
	"fmt"
	"errors"
	//"bufio"
	//"strings"
	"sync"
	"io"
	"syscall"
	"os"
	"os/signal"
	"container/list"
	"log"
	"encoding/binary"
	"strconv"
)


//Loggers
var (
	infoLog *log.Logger
	traceLog   *log.Logger
    warnLog *log.Logger
    errLog   *log.Logger
    dbgLog   *log.Logger
) 

var debugEn bool = true

var connection *Connection

//upStrms is from client to server
//downStrms is from server to client
type Connection struct{
	upStrms, downStrms []Stream
	upStrmSendChan, upStrmRecvChan, downStrmSendChan, downStrmRcvChan chan int
	upStrmBuf, downStrmBuf IntrConnBuf
	streamBufs map[Stream]IntrStrBuf
	nextStream, numStream int
	upStreamLastSent, upStreamLastRecv, downStreamLastSent, downStreamLastRecv uint32
	nextSeq uint32 
	hopNum int
	nextHopAddr []string
	//mutex* sync.Mutex
}

func newConnection(hopNumber int/*, something to indicate next hop*/) *Connection{
	conn := new(Connection)
	conn.nextSeq = 0
	conn.hopNum = hopNumber
	conn.streamBufs = make(map[Stream]IntrStrBuf)
	conn.upStrmBuf = *NewIntrConnBuf(true)
	conn.downStrmBuf = *NewIntrConnBuf(false)
	return conn
}

func (conn Connection) addStream(stream Stream) (){
	strmBuf := NewIntrStrBuf(true)
	conn.streamBufs[stream] = *strmBuf
}

func (conn Connection) createPacket(data []byte, seqNo uint32) *Packet{
	header := make([]byte, 2)
	binary.LittleEndian.PutUint32(header, seqNo)
	packet := NewPacket(header, data)
	return packet
}

//Direction: client to server
func (conn Connection) handleUpStream(){
	conn.upStream(conn.upStrmSendChan, conn.upStrmRecvChan)
}

func (conn Connection) upStream(upSendChan, upRecvChan chan int) error{
	
	errChan := make(chan error, 2)

	var wg sync.WaitGroup
	wg.Add(2)


	//Send routine
	go func() {
		_ = <- conn.upStrmBuf.consProdChan
		conn.upStrmBuf.mutex.Lock()
		for len(conn.upStrmBuf.getNext()) > 0 {
			read := conn.upStrmBuf.bufList.Front().Value.(Packet).toByte()
			if(len(read)==0) {
				errChan <- io.EOF
				return
			}
			if debugEn {
				dbgLog.Println("R: %s!\n\n\n",read)
				Dump(read)
				dbgLog.Println("\nRlen: %d!\n",len(read))
				}
			//Round robin
			conn.upStrms[conn.nextStream].sendPacketBytes(read, true)
			conn.nextStream = (conn.nextStream+1) % conn.numStream
		}
		conn.upStrmBuf.mutex.Unlock()
	} ()

	//Receive routine
	go func() {
		for {
			_= <- conn.upStrmRecvChan
			dbgLog.Println("Got a packet")
			//TODO: Query all streams and copy from their buffers to internal buf
		}

	}()

	wg.Wait()
	if len(errChan) > 0 {
		infoLog.Println("Closing connection")
		return <-errChan
	}

	return nil
}

//Direction: server to client
func (conn Connection) downStream() (){
	
}


////STREAM Interface
type Stream struct{
	conn *Connection
	upStrmBuf, downStrmBuf IntrStrBuf
}

func newStream(conn *Connection) *Stream{
	stream := new(Stream)
	stream.conn = conn
	//upStrmBuf := NewIntrStrBuf(true)
	//downStrmBuf := NewIntrStrBuf(false)
	return stream
}

func (str Stream) sendPacketBytes(packet []byte, isUpStream bool)(){
	if isUpStream {
		//str.upStrmBuf.consProdChan <- true 
	}
}


//Packet interface
var pktHeaderSize int = 4

type Packet struct{
	sequence, length, payload []byte
	totalLength uint32
}

func (p Packet) getSequence() uint32{
	return binary.BigEndian.Uint32(p.sequence)
}

func (p Packet) toByte() []byte{
	//totalLength := len(p.sequence) + len(p.length) + len(p.payload)
	tmp := append(p.sequence, p.length...)
	data := append(tmp, p.payload...)
	return data
}

func NewPacket(sequence, payload []byte) *Packet{
	p := new(Packet)
	p.sequence = sequence
	p.payload = payload
	//p.length = uint32(len(header) + len(data))
	binary.LittleEndian.PutUint32(p.length, uint32(len(sequence) + len(payload))) 
	return p
}

//Internal buffer interface 
//TODO:(Make the two inherit the same class)
type IntrConnBuf struct {
	bufList* list.List
	consProdChan chan bool
	mutex* sync.Mutex
	addHdr, remvHdr bool
	isUpstream bool
	debug bool
}

func NewIntrConnBuf(upStream bool) *IntrConnBuf{
	p := new(IntrConnBuf)
	p.bufList = list.New()
	p.consProdChan = make(chan bool, 1)
	p.mutex = &sync.Mutex{}
	p.isUpstream = upStream
	return p
}

func (buf IntrConnBuf) getNext() []Packet{
	for e := buf.bufList.Front(); e != nil; e = e.Next() {
		seq := e.Value.(Packet).getSequence()
		if seq == connection.nextSeq {
			data := make([]Packet, 1)
			return data
		}
	}
	data := make([]Packet, 0)
	return data

}


type IntrStrBuf struct {
	str* Stream
	//TODO: Replace with heap
	bufList* list.List
	consProdChan chan bool
	mutex* sync.Mutex
	addHdr, remvHdr bool
	isUpstream bool
	debug bool
}

func NewIntrStrBuf(upStream bool) *IntrStrBuf{
	p := new(IntrStrBuf)
	p.debug = true
	p.bufList = list.New()
	p.consProdChan = make(chan bool, 1)
	p.mutex = &sync.Mutex{}
	p.isUpstream = upStream
	return p
}


// Read reads data from the connection.
// Read can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (ib IntrConnBuf) Read(b []byte) (n int, err error){
	dbgLog.Println("Waiting to Read: %s!\n\n\n",b)
	chanErr:= <- ib.consProdChan
	ib.mutex.Lock()
	defer ib.mutex.Unlock()
	
	if chanErr!= true {
		err = errors.New("Channel Error")
		errLog.Println("Channel Error!")
		return 0, io.EOF
	}
 
	read := ib.bufList.Front().Value.(Packet).toByte()
	if(len(read)==0) {return 0, io.EOF}
	for i, p := range read {
		b[i] = p
	}

	ib.bufList.Remove(ib.bufList.Front())
	
	if ib.debug {
		dbgLog.Println("IntrConnBuf:Read")
		dbgLog.Println("R: %s!\n\n\n",b)
		dbgLog.Println("\nRlen: %d!\n",len(read))
	}

	return len(read), err
}

// Write writes data to the connection.
// Write can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (ib IntrConnBuf) Write(b []byte) (n int, err error){
	ib.mutex.Lock()
	defer ib.mutex.Unlock()
	
	if(len(b)==0) {return 0, io.EOF}
	NewPacket(b[0:pktHeaderSize - 1], b[pktHeaderSize: len(b) - 1]) 
	ib.bufList.PushBack(b)
	
//	if ib.isUpstream {
//		ib.str.conn.upStreamChan <- true
//	} else {
//		ib.str.conn.downStreamChan <- true
//	}
	
	if ib.debug {
		dbgLog.Println("IntrConnBuf:Write")
		dbgLog.Println("W: %s!\n\n\n",b)
		dbgLog.Println("\nWlen: %d!\n",len(b))
	}

	ib.consProdChan <- true
	return len(b), err
}

// Read reads data from the connection.
// Read can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (ib IntrStrBuf) Read(b []byte) (n int, err error){
	
	/*
	dbgLog.Println("Waiting to Read: %s!\n\n\n",b)
	chanErr:= <- ib.consProdChan
	ib.mutex.Lock()
	defer ib.mutex.Unlock()
	
	if chanErr!= true {
		err = errors.New("Channel Error")
		errLog.Println("Channel Error!")
		return 0, io.EOF
	}
 
	read := ib.bufList.Front().Value.(Packet).toByte()
	if(len(read)==0) {return 0, io.EOF}
	for i, p := range read {
		b[i] = p
	}

	ib.bufList.Remove(ib.bufList.Front())
	
	if ib.debug {
		dbgLog.Println("IntrStrBuf:Read")
		dbgLog.Println("R: %s!\n\n\n",b)
		dbgLog.Println("\nRlen: %d!\n",len(read))
	}

	return len(read), err
	*/
	return 0, err
}

// Write writes data to the connection.
// Write can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (ib IntrStrBuf) Write(b []byte) (n int, err error){
	ib.mutex.Lock()
	defer ib.mutex.Unlock()
	
	if(len(b)==0) {return 0, io.EOF}
	NewPacket(b[0:pktHeaderSize - 1], b[pktHeaderSize: len(b) - 1]) 
	ib.bufList.PushBack(b)
	
//	if ib.isUpstream {
//		ib.str.conn.upStreamChan <- true
//	} else {
//		ib.str.conn.downStreamChan <- true
//	}
	
	if ib.debug {
		dbgLog.Println("IntrStrBuf:Write")
		dbgLog.Println("W: %s!\n\n\n",b)
		dbgLog.Println("\nWlen: %d!\n",len(b))
	}

	return len(b), err
}


var handlerChan = make(chan int)

func copyLoop(aArr/*, bArr */[]net.Conn, a2bBuff IntrConnBuf, b2aBuff IntrStrBuf) error {
// Note: b is always the pt connection.  a is the SOCKS/ORPort connection.
	errChan := make(chan error, 2)

	var wg sync.WaitGroup
	wg.Add(2)

/*
	go func() {
		defer wg.Done()
		defer bArr[0].Close()
		defer aArr[0].Close()
		_, err := io.Copy(bArr[0], a2bBuff)

		errChan <- err
	}()

	go func() {
		defer wg.Done()
		defer bArr[0].Close()
		defer aArr[0].Close()
		_, err := io.Copy(a2bBuff, aArr[0])
		errChan <- err
	}()

	go func() {
		defer wg.Done()
		defer aArr[0].Close()
		defer bArr[0].Close()
		_, err := io.Copy(aArr[0], b2aBuff)

		errChan <- err
	}()

	go func() {
		defer wg.Done()
		defer aArr[0].Close()
		defer bArr[0].Close()
		_, err := io.Copy(b2aBuff, bArr[0])
		errChan <- err
	}()

*/
	go func() {
		defer wg.Done()
		defer aArr[0].Close()
		_, err := io.Copy(a2bBuff, aArr[0])
		errChan <- err
	}()


	go func() {
		defer wg.Done()
		defer aArr[0].Close()
		_, err := io.Copy(aArr[0], b2aBuff)
		errChan <- err
	}()


// Wait for both upstream and downstream to close.  Since one side
// terminating closes the other, the second error in the channel will be
// something like EINVAL (though io.Copy() will swallow EOF), so only the
// first error is returned.
		wg.Wait()
		if len(errChan) > 0 {
			infoLog.Println("Closing connection")
			return <-errChan
		}

		return nil
	}
func handler(conn *pt.SocksConn) error {
	infoLog.Println("Handler!")
	defer conn.Close()
	remote, err := net.Dial("tcp", conn.Req.Target)
	if err != nil {
		conn.Reject()
		return err
	}
	defer remote.Close()
	err = conn.Grant(remote.RemoteAddr().(*net.TCPAddr))
	if err != nil {
		return err
	}

	var newStream Stream
	connection.addStream(newStream)
	
	copyLoop([]net.Conn {conn},/* []net.Conn {remote},*/ connection.upStrmBuf, newStream.upStrmBuf)
	

	return nil
}

func acceptLoop(ln *pt.SocksListener) error {
	defer ln.Close()
	for {
		infoLog.Println("Accepting!")
		conn, err := ln.AcceptSocks()
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Temporary() {
				dbgLog.Println("Error!")
				continue
			}
			return err
		}
		go func() {
			err := handler(conn)
			if err != nil {
				errLog.Println("handler error: %s", err)
			}
		}()
	}
}



func intiateLogger(){
	traceLog = log.New(os.Stdout,"Trace: ",log.Ldate|log.Ltime|log.Lshortfile)
	infoLog = log.New(os.Stdout,"INFO: ",log.Ldate|log.Ltime|log.Lshortfile)
	errLog = log.New(os.Stderr,"Error: ",log.Ldate|log.Ltime|log.Lshortfile)
	warnLog = log.New(os.Stdout,"Warn: ",log.Ldate|log.Ltime|log.Lshortfile)
	dbgLog = log.New(os.Stdout,"Warn: ",log.Ldate|log.Ltime|log.Lshortfile)

}

func main() {
	intiateLogger()
	go StartServer()
	var err error
// 		var ptInfo pt.ClientInfo

	hopNum, err := strconv.Atoi((os.Args[1]))
	if err != nil {
		errLog.Println("Argument error!")
	}
	connection = newConnection(hopNum)
	go connection.handleUpStream()
	
	listeners := make([]net.Listener, 0)

	ln, err := pt.ListenSocks("tcp", "127.0.0.1:44444")

	if err != nil {
		errLog.Println("Error!")
	}
	go acceptLoop(ln)
	pt.Cmethod("Main", ln.Version(), ln.Addr())
	listeners = append(listeners, ln)
	pt.CmethodsDone()

	var numHandlers int = 0
	var sig os.Signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

// wait for first signal
	sig = nil
	for sig == nil {
		select {
		case n := <-handlerChan:
			numHandlers += n
		case sig = <-sigChan:
		}
	}
	for _, ln := range listeners {
		ln.Close()
	}

	if syscall.SIGTERM == sig || syscall.SIGINT == sig {
		return
	}

// wait for second signal or no more handlers
	sig = nil
	for sig == nil && numHandlers != 0 {
		select {
		case n := <-handlerChan:
			numHandlers += n
		case sig = <-sigChan:
		}
	}
	fmt.Printf("Done!Done!")
}





/*
// Source: https://github.com/glycerine/golang-hex-dumper
// hexdump: a hexdumper utility written in Golang
//
// Copyright 2015 Jason E. Aten <j.e.aten -a-t- g-m-a-i-l dot c-o-m>
// License: MIT
*/
//package hex

//import "fmt"

func Dump(by []byte) {
	n := len(by)
	rowcount := 0
	stop := (n / 8) * 8
	k := 0
	for i := 0; i <= stop; i += 8 {
		k++
		if i+8 < n {
			rowcount = 8
		} else {
			rowcount = min(k*8, n) % 8
		}

		fmt.Printf("pos %02d  hex:  ", i)
		for j := 0; j < rowcount; j++ {
			fmt.Printf("%02x  ", by[i+j])
		}
		for j := rowcount; j < 8; j++ {
			fmt.Printf("    ")
		}
		fmt.Printf("  '%s'\n", viewString(by[i:(i+rowcount)]))
	}

}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func viewString(b []byte) string {
	r := []rune(string(b))
	for i := range r {
		if r[i] < 32 || r[i] > 126 {
			r[i] = '.'
		}
	}
	return string(r)
}
