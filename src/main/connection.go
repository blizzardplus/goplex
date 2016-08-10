package main

import (
	"sync"
	"container/list"
	"net"
)

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
