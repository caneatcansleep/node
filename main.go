package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
	"unsafe"
)

type IP []byte

type RelayInfo struct {
	PreHop      IP
	NextHop     IP
	ListenPort  int
	NextHopPort int
}

type TcpAddr struct {
	Ip   IP
	Port int
}

var connCh chan net.Conn

func init() {
	connCh = make(chan net.Conn)
}

func (ip IP) String() string {
	tmp := make([]byte, 7)
	for i := 0; i < 3; i++ {
		tmp = append(tmp, ip[i])
		tmp = append(tmp, '.')
	}
	tmp = append(tmp, ip[3])
	return string(tmp)
}

func ConnectToNextHop(relayInfo RelayInfo) {

	var rightConn net.Conn
	var err error

	nextHop := relayInfo.NextHop.String() + strconv.Itoa(relayInfo.NextHopPort)
	fmt.Println("nextHop = ", nextHop)

	// 有可能下一跳还没有监听，因此我们需要不断尝试建立tcp连接，直到建立成功
	for i := 0; i < 10; i++ {
		rightConn, err = net.Dial("tcp", nextHop)
		if err != nil {
			fmt.Printf("failed to Dial %d times, error = %v\n", i, err)
			time.Sleep(3 * time.Second)
			continue
		}
		break
	}
	// 如果超过10次还没有建立成功，直接返回
	if err != nil {
		fmt.Printf("failed to Dial over %d times, exist!\n", 10)
		return
	}

	// 等待上一跳中继节点向我们建立tcp连接
	leftConn := <-connCh

	defer rightConn.Close()
	defer leftConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		for {
			_, err := io.Copy(leftConn, rightConn) // rightConn -> leftConn
			if err != nil {
				fmt.Printf("io.Copy failed in 1, error = %v, exit!\n", err)
				wg.Done()
				return
			}
		}
	}()

	go func() {
		for {
			_, err := io.Copy(rightConn, leftConn) // leftConn -> rightConn
			if err != nil {
				fmt.Printf("io.Copy failed in 2, error = %v, exit!\n", err)
				wg.Done()
				return
			}
		}
	}()
	wg.Wait()
}

// TCP Server端测试
// 处理函数
func ProcessMsgFromController(connController net.Conn) {
	defer connController.Close() // 关闭连接
	reader := bufio.NewReader(connController)
	var buf [128]byte
	n, err := reader.Read(buf[:]) // 读取数据
	if err != nil {
		fmt.Println("read from controller failed, error = ", err)
		return
	}
	// 这里可能有问题，关键是Sizeof函数的作用可能不是我理解的那样
	if n < int(unsafe.Sizeof(RelayInfo{})) {
		fmt.Printf("read %d byte from controller, less than sizeof(RelayInfo)\n", n)
		return
	}
	relayInfo := RelayInfo{}
	relayInfo.PreHop = buf[:4]
	relayInfo.NextHop = buf[4:8]
	relayInfo.ListenPort = int(binary.LittleEndian.Uint32(buf[8:10]))
	relayInfo.NextHopPort = int(binary.LittleEndian.Uint32(buf[10:12]))
	fmt.Println("relayInfo = ", relayInfo)
	ConnectToNextHop(relayInfo)
}

func ListenToController() {
	listen, err := net.Listen("tcp", "192.168.19.136:8080")
	if err != nil {
		fmt.Println("ListenToController: Listen() failed, error = ", err)
		return
	} else {
		fmt.Println("ListenToController: Listen to controller success!")
	}

	for {
		conn, err := listen.Accept() // 监听客户端的连接请求
		if err != nil {
			fmt.Println("ListenToController: Accept() failed, error = ", err)
			continue
		}
		go ProcessMsgFromController(conn) // 启动一个goroutine来处理客户端的连接请求
	}
}

func ListenToLeft(address string) {
	listen, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Printf("Listen at %s failed, error = %v\n", address, err)
		return
	} else {
		fmt.Printf("Listen at %s, wait pre hop to connect us.\n", address)
	}

	for {
		connLeft, err := listen.Accept() // 监听客户端的连接请求
		if err != nil {
			fmt.Println("ListenToLeft: Accept() failed, error = ", err)
			continue
		}
		fmt.Println("ListenToLeft: Accept from pre hop, with client address = ", connLeft.RemoteAddr().String())
		connCh <- connLeft
	}
}

func main() {
	go ListenToController()
	address := "xx.xx.xx.xx:8080"
	ListenToLeft(address)
}
