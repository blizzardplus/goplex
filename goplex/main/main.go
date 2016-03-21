package main 

	import (
	"pt"
	"net"
	"fmt"
	"errors"
	//"bufio"
	//"strings"
	"sync"
    //"sync/atomic"
	"io"
	"syscall"
	"os"
	"os/signal"
	"container/list"
//	"bufio"
)


type InternalBuf struct {
	bufList* list.List
	consProdChan chan bool
	mutex* sync.Mutex
	debug bool
}

func NewIntBuff() *InternalBuf{
	p := new(InternalBuf)
	p.bufList = list.New()
	p.consProdChan = make(chan bool, 1)
	p.mutex = &sync.Mutex{}
	return p
}

// Read reads data from the connection.
// Read can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (ib InternalBuf) Read(b []byte) (n int, err error){
	//fmt.Printf("Waiting to Read: %s!\n\n\n",b)
	chanErr:= <- ib.consProdChan
	ib.mutex.Lock()
	if chanErr!= true {
		err = errors.New("Channel Error")
		fmt.Printf("Error!")
		return 0, io.EOF
	}

	read := ib.bufList.Front().Value.([]byte)
	if(len(read)==0) {return 0, io.EOF}
	for i, p := range read {
		b[i] = p
	}

	ib.bufList.Remove(ib.bufList.Front())
	if ib.debug {
		//fmt.Printf("R: %s!\n\n\n",b)
		fmt.Printf("\nRlen: %d!\n",len(read))
		}
	ib.mutex.Unlock()

	return len(read), err
}

// Write writes data to the connection.
// Write can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (ib InternalBuf) Write(b []byte) (n int, err error){
	ib.mutex.Lock()
	defer ib.mutex.Unlock()
	if(len(b)==0) {return 0, io.EOF}
	ib.bufList.PushBack(b)
	ib.consProdChan <- true
	if ib.debug {
		 //fmt.Printf("W: %s!\n\n\n",b)
		 fmt.Printf("\nWlen: %d!\n",len(b))
		 //fmt.Printf("len: %d!\n\n\n",len(ib.bufList.Front().Value.([]byte)))
	}

	return len(b), err
}


var handlerChan = make(chan int)


func copyLoop(aArr, bArr []net.Conn, a2bBuff, b2aBuff *InternalBuf) error {
// Note: b is always the pt connection.  a is the SOCKS/ORPort connection.
	errChan := make(chan error, 4)

	var wg sync.WaitGroup
	wg.Add(4)

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



// Wait for both upstream and downstream to close.  Since one side
// terminating closes the other, the second error in the channel will be
// something like EINVAL (though io.Copy() will swallow EOF), so only the
// first error is returned.
		wg.Wait()
		if len(errChan) > 0 {
			fmt.Print("Closing connection")
			return <-errChan
		}

		return nil
	}
func handler(conn *pt.SocksConn) error {
	fmt.Printf("Handler!")
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
	a2bBuff := NewIntBuff()
	a2bBuff.debug = true
	b2aBuff := NewIntBuff()
	b2aBuff.debug = true
	copyLoop([]net.Conn {conn}, []net.Conn {remote}, a2bBuff, b2aBuff)

	return nil
}

func acceptLoop(ln *pt.SocksListener) error {
	defer ln.Close()
	for {
		fmt.Printf("Accepting!")
		conn, err := ln.AcceptSocks()
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Temporary() {
				continue
			}
			return err
		}
		go func() {
			err := handler(conn)
			if err != nil {
				fmt.Printf("handler error: %s", err)
			}
		}()
	}
}
func main() {
	go StartServer()
	var err error
// 		var ptInfo pt.ClientInfo
	listeners := make([]net.Listener, 0)

	ln, err := pt.ListenSocks("tcp", "127.0.0.1:55555")

	if err != nil {
		fmt.Printf("Error!")
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
