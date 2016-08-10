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
	"net/url"
	"golang.org/x/net/proxy"
	"time"
)


//Loggers
var (
	infoLog *log.Logger
	traceLog   *log.Logger
    warnLog *log.Logger
    errLog   *log.Logger
    dbgLog   *log.Logger
    tmpLogOut  *os.File
) 

var debugEn bool = false

//var connection *Connection

//upStrms is from client to server
//downStrms is from server to client
type Connection struct{
	upStrms, downStrms []Stream
	upStrmSendChan, upStrmRecvChan, downStrmSendChan, downStrmRcvChan chan int
	upStrmBuf, downStrmBuf *IntrConnBuf
	upStreamBufs, downStreamBufs map[int]*IntrStrBuf
	nextUpStream, numUpStream, nextDownStream, numDownStream int
	upStreamLastSent, upStreamLastRecv, downStreamLastSent, downStreamLastRecv uint32
	nextSeqToSend uint32 
	hopNum int
	nextHopAddr []string
	isConnToEndHostEstabl bool
	remoteConn net.Conn
	connID uint32
	mutex* sync.Mutex
	
	upConsProdChan, downConsProdChan chan Packet
	upStreamQ, downStreamQ *list.List
	upStreamQMutex, downStreamQMutex *sync.Mutex
	upStreamNextToSend, downStreamNextToSend uint32
}

func newConnection(hopNumber int/*, something to indicate next hop*/) *Connection{
	conn := new(Connection)
	conn.nextSeqToSend = 0
	conn.nextUpStream = 0
	conn.nextDownStream = 0
	conn.hopNum = hopNumber
	conn.upStreamBufs = make(map[int]*IntrStrBuf)
	conn.downStreamBufs = make(map[int]*IntrStrBuf)
	conn.upStrmBuf = NewIntrConnBuf(true, conn)
	conn.downStrmBuf = NewIntrConnBuf(false, conn)
	conn.isConnToEndHostEstabl = false
	conn.mutex = &sync.Mutex{}

	return conn
}

func (conn *Connection) addUpStream(stream *Stream) (){
	conn.upStreamBufs[conn.numUpStream] = stream.strmBuf
	conn.numUpStream++
}

func (conn *Connection) addDownStream(stream *Stream) (){
	conn.downStreamBufs[conn.numDownStream] = stream.strmBuf
	conn.numDownStream++
}

func (conn *Connection) createPacket(data []byte, seqNo uint32) Packet{
	packet := NewPacketWithSeq(seqNo, data)
	return packet
}

func (conn *Connection) getStrmBuf(isUpStream bool) *IntrConnBuf{
	if isUpStream { 
		return conn.upStrmBuf
	}else
	{
		return conn.downStrmBuf
	}
}

func (conn *Connection) getNextStream(isUpStream bool) int{
	if isUpStream { 
		return conn.nextUpStream
	}else
	{
		return conn.nextDownStream
	}
}

func (conn *Connection) setNextStream(isUpStream bool, val int) {
	if isUpStream { 
		conn.nextUpStream = val
	}else
	{
		conn.nextDownStream = val
	}
}

func (conn *Connection) getNumStreams(isUpStream bool) int{
	if isUpStream { 
		return conn.numUpStream
	}else
	{
		return conn.numDownStream
	}
}


func (conn *Connection) getStreamBufs(isUpStream bool) map[int]*IntrStrBuf{
	if isUpStream { 
		return conn.upStreamBufs
	}else
	{
		return conn.downStreamBufs
	}
}



//Direction: client to server
func (conn *Connection) handleStreams(){
	go conn.handleStream(true)
	go conn.handleStream(false)
}


func (conn *Connection) handleStream(isUpStream bool) error{
	
	errChan := make(chan error, 1)

	var wg sync.WaitGroup
	wg.Add(1)
	

	go func() {
		for {
			_ = <- conn.getStrmBuf(isUpStream).consProdChan
			
			//conn.getStrmBuf(isUpStream).mutex.Lock()
			//conn.mutex.Lock()
			
			if (conn.hopNum == 4) || (conn.hopNum == 1) ||  (conn.hopNum == 2) ||  (conn.hopNum == 3){
				continue
			}

			nextPacket := conn.getStrmBuf(isUpStream).getNext()

			for len(nextPacket) > 0 {
				read := nextPacket
				
				if debugEn {
					dbgLog.Println("R: %s!\n\n\n",read)
					//Dump(read)
					dbgLog.Println("\nRlen: %d!\n",len(read))
				}
////				if conn.hopNum==4 {
////					dbgLog.Print("Sending packet to stream")
////					dbgLog.Print(conn.getNextStream(isUpStream)) 
////				}
				//Round robin
				conn.getStreamBufs(isUpStream)[conn.getNextStream(isUpStream)].bufList.PushBack(read)
				conn.getStreamBufs(isUpStream)[conn.getNextStream(isUpStream)].consProdChan <- true
				conn.setNextStream(isUpStream, (conn.getNextStream(isUpStream)+1) % conn.getNumStreams(isUpStream))
				
////				if conn.hopNum==4 {
////					dbgLog.Print("Next stream")
////					dbgLog.Print(conn.getNextStream(isUpStream)) 
////				}
				//Start over
				nextPacket = conn.getStrmBuf(isUpStream).getNext()
			}
			//conn.getStrmBuf(isUpStream).mutex.Unlock()
			//conn.mutex.Unlock()
		}
	} ()


	wg.Wait()
	if len(errChan) > 0 {
		infoLog.Println("Closing connection")
		return <-errChan
	}

	return nil
}

//Direction: server to client
func (conn *Connection) downStream() (){
	
}


////STREAM Interface
type Stream struct{
	conn *Connection
	strmBuf *IntrStrBuf
	streamID int
}


var streamCntr = 0
func newStream(conn *Connection) *Stream{
	stream := new(Stream)
	strmBuf := NewIntrStrBuf(true, stream)
	stream.strmBuf = strmBuf
	stream.conn = conn
	stream.streamID = streamCntr 
	streamCntr++ 
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
const pktSeqSize = 4
const pktLenSize = 4

type Packet struct{
	sequence, length, payload []byte
	totalLength uint32
}

func (p Packet) getSequence() uint32{
	return binary.BigEndian.Uint32(p.sequence)
}

func (p Packet) getLength() uint32{
	return binary.BigEndian.Uint32(p.length)
}

func (p Packet) toByte() []byte{
	//totalLength := len(p.sequence) + len(p.length) + len(p.payload)
	tmp := append(p.sequence, p.length...)
	data := append(tmp, p.payload...)
	return data
}

func NewPacketFromBytes(bytes []byte) Packet{
	//TODO: Probably a better implementation since we already have the byte slice
	//Something like union is better
	p := NewPacketWithSeq(binary.BigEndian.Uint32(bytes[0:pktSeqSize]),  bytes[pktSeqSize+pktLenSize :])
	return p
}
func NewPacketWithSeq(sequence uint32, payload []byte) Packet{
	p := new(Packet)
	p.sequence = make([]byte, pktSeqSize)
	p.length = make([]byte, pktLenSize)
	binary.BigEndian.PutUint32(p.sequence, sequence) 
	p.payload = payload
	//p.length = uint32(len(header) + len(data))
	//Length is an override
	binary.BigEndian.PutUint32(p.length, sequence + uint32(len(payload))) 
	return *p
}

//Internal buffer interface 
//TODO:(Make the two inherit the same class)
type IntrConnBuf struct {
	conn *Connection
	bufList* list.List
	consProdChan chan bool
tempConsProdChan chan bool
nextToDump uint32
	mutex* sync.Mutex
	bufferMutex* sync.Mutex
	addHdr, remvHdr bool
	isUpStream bool
	nextSeqExpected uint32
	debug bool
}

func NewIntrConnBuf(upStream bool, conn *Connection) *IntrConnBuf{
	p := new(IntrConnBuf)
	p.conn = conn
	p.bufList = list.New()
	p.consProdChan = make(chan bool)
p.tempConsProdChan = make(chan bool)
p.nextToDump = 0
	p.mutex = &sync.Mutex{}
	p.bufferMutex = &sync.Mutex{}
	p.isUpStream = upStream
	p.nextSeqExpected = 0
	return p
}

func (buf *IntrConnBuf) getNext() []byte{
	var data []byte	
	
//if !buf.isUpstream{
//data = make([]byte, 0)
//return data
//}

	buf.bufferMutex.Lock()
	for e := buf.bufList.Front(); e != nil; e = e.Next() {
		
		packet := e.Value.(Packet)

		if (buf.conn.hopNum == 2 || buf.conn.hopNum == 3){
			buf.bufList.Remove(e)
			buf.bufferMutex.Unlock()
			data = packet.toByte()
			return data
		}
		seq := packet.getSequence()
		if seq == buf.nextSeqExpected {
			buf.bufList.Remove(e)
			buf.nextSeqExpected += (packet.getLength() + 1)
			buf.bufferMutex.Unlock()
			//data := make([]byte, 1)
			if(buf.conn.hopNum == 4 && buf.isUpStream) || (buf.conn.hopNum == 1 && (!buf.isUpStream)){
				data = packet.payload
			}else {
				data = packet.toByte()
			}
			
			if !buf.isUpStream && buf.conn.hopNum == 4 {
				fmt.Println("Packet " + fmt.Sprint(packet.getSequence()) + " of size " + fmt.Sprint(packet.getLength())+":\n\n")
				Dump(packet.payload)
				tmpLogOut.WriteString("Packet " + fmt.Sprint(packet.getSequence()) + " of size " + fmt.Sprint(packet.getLength())+":\n\n")
				tmpLogOut.Write(packet.payload)
				tmpLogOut.WriteString("\n\n--------\n\n")
			}
			
			return data
		}
	}

	data = make([]byte, 0)
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

func NewIntrStrBuf(upStream bool, str *Stream) *IntrStrBuf{
	p := new(IntrStrBuf)
	p.debug = true
	p.str = str
	p.bufList = list.New()
	p.consProdChan = make(chan bool, 1)
	p.mutex = &sync.Mutex{}
	p.isUpstream = upStream
	return p
}


// Read reads data from the connection.
// Read can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (ib *IntrConnBuf) Read(b []byte) (n int, err error){
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
		dbgLog.Println("IntrConnBuf:Read")
		dbgLog.Println("R: %s!\n\n\n",b)
		dbgLog.Println("\nRlen: %d!\n",len(read))
	}

	return len(read), err
	*/
	return 0, err
}

func (ib *IntrConnBuf) Dumper() (){
	//fmt.Println("in Dumper")
	<-ib.tempConsProdChan
	var read []byte
	
	if (ib.conn.hopNum == 2 || ib.conn.hopNum == 3){
		ib.mutex.Lock()
		for e := ib.bufList.Front(); e != nil; e = e.Next() {
			packet := e.Value.(Packet)
			ib.bufList.Remove(e)
			read = packet.toByte()
			
			if(!ib.isUpStream){
				tmpLogOut.WriteString("Next Packet " + fmt.Sprint(packet.getSequence()) + " of size " + fmt.Sprint(packet.getLength())+":\n\n")
				tmpLogOut.Write(packet.payload)
				tmpLogOut.WriteString("\n\n--------\n\n")
			}
			
			//Round robin
			ib.conn.getStreamBufs(ib.isUpStream)[ib.conn.getNextStream(ib.isUpStream)].bufList.PushBack(read)
			ib.conn.getStreamBufs(ib.isUpStream)[ib.conn.getNextStream(ib.isUpStream)].consProdChan <- true
			ib.conn.setNextStream(ib.isUpStream, (ib.conn.getNextStream(ib.isUpStream)+1) % ib.conn.getNumStreams(ib.isUpStream))
		}
		ib.mutex.Unlock()
		return
	}
	
	if (ib.conn.hopNum == 4) || (ib.conn.hopNum == 1){
		ib.mutex.Lock()
		
		for e := ib.bufList.Front(); e != nil; e = e.Next() {
			packet := e.Value.(Packet)
		
			seq := packet.getSequence()
			if seq == ib.nextToDump {
				
				if(!ib.isUpStream){
				tmpLogOut.WriteString("Next Packet " + fmt.Sprint(packet.getSequence()) + " of size " + fmt.Sprint(packet.getLength())+":\n\n")
				tmpLogOut.Write(packet.payload)
				tmpLogOut.WriteString("\n\n--------\n\n")
				}

				ib.bufList.Remove(e)

				ib.nextToDump += (packet.getLength() + 1)
				
				
				if(ib.conn.hopNum == 4 && ib.isUpStream) || (ib.conn.hopNum == 1 && (!ib.isUpStream)){
					read = packet.payload
				}else {
					read = packet.toByte()
				}

				//Round robin
				ib.conn.getStreamBufs(ib.isUpStream)[ib.conn.getNextStream(ib.isUpStream)].bufList.PushBack(read)
				ib.conn.getStreamBufs(ib.isUpStream)[ib.conn.getNextStream(ib.isUpStream)].consProdChan <- true
				ib.conn.setNextStream(ib.isUpStream, (ib.conn.getNextStream(ib.isUpStream)+1) % ib.conn.getNumStreams(ib.isUpStream))
			}
		}
		
		ib.mutex.Unlock()
		
		
	}
	
	
	
}

// Write writes data to the connection.

// Write can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (ib *IntrConnBuf) Write(b []byte) (n int, err error){
	
	//ib.conn.mutex.Lock()
	
	//defer ib.conn.mutex.Unlock()

	
	if(len(b)==0) {return 0, io.EOF}
	
	if (ib.conn.hopNum == 1 && ib.isUpStream) || (ib.conn.hopNum == 4 && (!ib.isUpStream)) {
		ib.mutex.Lock()
		seq := ib.conn.nextSeqToSend
		packet := NewPacketWithSeq(seq, b)
		ib.conn.nextSeqToSend += packet.getLength() + 1  
		
		ib.bufferMutex.Lock()
		ib.bufList.PushBack(packet)
		ib.bufferMutex.Unlock()
		
		ib.mutex.Unlock()
		
	} else{
		//packet := NewPacket(seq, b[pktHeaderSize: len(b) - 1])
		ib.mutex.Lock()
		packet := NewPacketFromBytes(b)
		
		//ib.conn.downConsProdChan <- packet
		
		ib.bufferMutex.Lock()
		ib.bufList.PushBack(packet)
		ib.bufferMutex.Unlock()
		
		ib.mutex.Unlock()
		
	}
	go ib.Dumper()	
	ib.tempConsProdChan <- true

	time.Sleep(time.Millisecond)
	
	ib.consProdChan <- true
	return len(b), err
}

// Read reads data from the connection.
// Read can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (ib *IntrStrBuf) Read(b []byte) (n int, err error){
	
	if debugEn { 
		dbgLog.Println("Waiting to Read on Stream: "+  strconv.Itoa(ib.str.streamID) + "\n\n")
	}
	chanErr:= <- ib.consProdChan
	ib.mutex.Lock()
	defer ib.mutex.Unlock()
	
	if chanErr!= true {
		err = errors.New("Channel Error")
		errLog.Println("Channel Error!")
		return 0, io.EOF
	}
 
	read := ib.bufList.Front().Value.([]byte)
	if(len(read)==0) {return 0, io.EOF}
	for i, p := range read {
		b[i] = p
	}

	ib.bufList.Remove(ib.bufList.Front())
	
	return len(read), err
}

// Write writes data to the connection.
// Write can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (ib *IntrStrBuf) Write(b []byte) (n int, err error){
	ib.mutex.Lock()
	defer ib.mutex.Unlock()
	
	if(len(b)==0) {return 0, io.EOF}
	//NewPacket(b[0:pktHeaderSize - 1], b[pktHeaderSize: len(b) - 1]) 
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



func regSocket2ConnBuf(connBuf *IntrConnBuf, sockets []net.Conn) error {
	errChan := make(chan error, 1)

	var wg sync.WaitGroup
	wg.Add(1)
	
	go func() {
		defer wg.Done()
		defer sockets[0].Close()
		_, err := io.Copy(connBuf, sockets[0])
		errChan <- err
	}()

	wg.Wait()
	if len(errChan) > 0 {

		infoLog.Println("Closing connection")
		return <-errChan
	}

	return nil
}

func regStrBuf2Socket(sockets []net.Conn, strBuf *IntrStrBuf) error {
// Note: b is always the pt connection.  a is the SOCKS/ORPort connection.
	errChan := make(chan error, 1)

	var wg sync.WaitGroup
	wg.Add(1)


	go func() {
		defer wg.Done()
		defer sockets[0].Close()
		_, err := io.Copy(sockets[0], strBuf)
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


func copyLoop(aArr/*, bArr */[]net.Conn, a2bBuff *IntrConnBuf, b2aBuff *IntrStrBuf) error {
// Note: b is always the pt connection.  a is the SOCKS/ORPort connection.
	errChan := make(chan error, 2)

	var wg sync.WaitGroup
	wg.Add(2)

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

var globalConnection *Connection
func handler(conn *pt.SocksConn, hopNum int) error {
	
	
	var connection *Connection
	if (hopNum == 4) && (globalConnection != nil){
		 connection = globalConnection
	}else {
		connection = newConnection(hopNum)
		globalConnection = connection
	}
	
	go connection.handleStreams()
	
	infoLog.Println("Handler!")
	if (connection.hopNum == 1) {
		defer conn.Close()
		//remote1, err1 := net.Dial("tcp", "127.0.0.1:33333")
		//remote2, err2 := net.Dial("tcp", "127.0.0.1:44444")
		
		u1, err1 := url.Parse("socks4://" + "127.0.0.1:33333")
		dialer1, err1 := proxy.FromURL(u1,proxy.Direct)
		remote1, err1 := dialer1.Dial("tcp",  conn.Req.Target )
		//remote2, err2 := net.Dial("tcp", "127.0.0.1:44444")
		
		
		u2, err2 := url.Parse("socks4://" + "127.0.0.1:44444")
		dialer2, err2 := proxy.FromURL(u2,proxy.Direct)
		remote2, err2 := dialer2.Dial("tcp", conn.Req.Target)
		
		
		if err1 != nil || err2 != nil {
			conn.Reject()
			if err1 != nil {return err1}
			if err2 != nil {return err2}
		}
		
		defer remote1.Close()
		defer remote2.Close()
		
		//Make new streams
		newStream1 := newStream(connection)
		connection.addUpStream(newStream1)
		
		newStream2 := newStream(connection)
		connection.addUpStream(newStream2)
		
		go regStrBuf2Socket([]net.Conn {remote1}, newStream1.strmBuf)
		go regSocket2ConnBuf(connection.downStrmBuf, []net.Conn {remote1})
		go regStrBuf2Socket([]net.Conn {remote2}, newStream2.strmBuf)
		go regSocket2ConnBuf(connection.downStrmBuf, []net.Conn {remote2})

		addr, err1:= net.ResolveTCPAddr("tcp","127.0.0.1:33333")
		err1 = conn.Grant(addr)
		if err1 != nil {
			return err1
		}
	} else if (connection.hopNum == 2 ||  connection.hopNum == 3 ){
		defer conn.Close()
		//remote, err := net.Dial("tcp", "127.0.0.1:55555")
		
		u, err := url.Parse("socks4://" + "127.0.0.1:55555")
		dialer, err := proxy.FromURL(u,proxy.Direct)
		remote, err := dialer.Dial("tcp",  conn.Req.Target )
		
		if err != nil {
			conn.Reject()
			return err
		}
		defer remote.Close()
		
		//Make new streams
		newStream := newStream(connection)
		connection.addUpStream(newStream)
		
		go regStrBuf2Socket([]net.Conn {remote}, newStream.strmBuf)
		go regSocket2ConnBuf(connection.downStrmBuf, []net.Conn {remote})
		
		addr, err:= net.ResolveTCPAddr("tcp","127.0.0.1:55555")
		err = conn.Grant(addr)
		if err != nil {
			return err
		}
	} else if (connection.hopNum == 4) {
		defer conn.Close()
		
		if !connection.isConnToEndHostEstabl{
			remote, err := net.Dial("tcp", conn.Req.Target)
			if err != nil {
				conn.Reject()
				return err
			} else{
				defer remote.Close()
				connection.isConnToEndHostEstabl = true
				connection.remoteConn = remote
			}
			
			//Make new streams
			newStream := newStream(connection)
			connection.addUpStream(newStream)
			 
			go regStrBuf2Socket([]net.Conn {remote}, newStream.strmBuf)
			go regSocket2ConnBuf(connection.downStrmBuf, []net.Conn {remote})
			
			err = conn.Grant(remote.RemoteAddr().(*net.TCPAddr))
			if err != nil {
				return err
			}
		}
		
		
		err := conn.Grant(connection.remoteConn.RemoteAddr().(*net.TCPAddr))
		if err != nil {
			return err
		}
	}

	newStream := newStream(connection)
	connection.addDownStream(newStream)
	//TODO: This should be a separate function that connects the socket, stream and connection buffer 
	copyLoop([]net.Conn {conn},/* []net.Conn {remote},*/ connection.upStrmBuf, newStream.strmBuf)
	//regStrBuf2Socket(newStream.strmBuf, []net.Conn {conn})
	

	return nil
}

func acceptLoop(ln *pt.SocksListener, hopNum int) error {
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
			err := handler(conn, hopNum)
			if err != nil {
				errLog.Println("handler error: %s", err)
			}
		}()
	}
}



func intiateLogger(){
	traceLog = log.New(os.Stdout,"Trace: ", log.Ldate|log.Ltime|log.Lshortfile)
	infoLog = log.New(os.Stdout,"INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	errLog = log.New(os.Stderr,"Error: ", log.Ldate|log.Ltime|log.Lshortfile)
	warnLog = log.New(os.Stdout,"Warn: ", log.Ldate|log.Ltime|log.Lshortfile)
	dbgLog = log.New(os.Stdout,"Debug: ", log.Ldate|log.Ltime|log.Lshortfile)
	
	
	hopNum := os.Args[1]
	//if hopNum == 4{
		var err error
		tmpLogOut, err = os.OpenFile("/tmp/out" + hopNum, os.O_RDWR | os.O_CREATE | os.O_APPEND, 0666)
		if err != nil {
		    fmt.Println("error opening file: %v", err)
		}
		
}


var handlerChan = make(chan int)
func main() {
	intiateLogger()
	pt.SocksInit()
	 
	//TODO: Implement REST server
	//go StartServer()
	var err error
// 		var ptInfo pt.ClientInfo

	hopNum, err := strconv.Atoi((os.Args[1]))
	if err != nil {
		errLog.Println("Argument error!")
	}
	
	listeners := make([]net.Listener, 0)
	
	var ln *pt.SocksListener
	if (hopNum == 1) {
		ln, err = pt.ListenSocks("tcp", "127.0.0.1:22222")
	} else if (hopNum == 2){
		ln, err = pt.ListenSocks("tcp", "127.0.0.1:33333")
	} else if (hopNum == 3){
		ln, err = pt.ListenSocks("tcp", "127.0.0.1:44444")
	} else if (hopNum == 4){
		ln, err = pt.ListenSocks("tcp", "127.0.0.1:55555")
	}


	if err != nil {
		errLog.Println("Error!")
	}
	go acceptLoop(ln, hopNum)
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
	//stop = 7
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
