package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"node/services"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var ListenToLeftAddr = "localhost:8082"
var ListenToControllerAddr = "localhost:8081"
var controllerAddr = "localhost:8080"
var connCh chan net.Conn

const digits = "0123456789abcdef"

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

func init() {
	connCh = make(chan net.Conn)
}

func (ip IP) string4() string {
	const max = len("255.255.255.255")
	ret := make([]byte, 0, max)
	ret = ip.appendTo4(ret)
	return string(ret)
}

func (ip IP) appendTo4(ret []byte) []byte {
	ret = appendDecimal(ret, ip[0])
	ret = append(ret, '.')
	ret = appendDecimal(ret, ip[1])
	ret = append(ret, '.')
	ret = appendDecimal(ret, ip[2])
	ret = append(ret, '.')
	ret = appendDecimal(ret, ip[3])
	return ret
}

// appendDecimal appends the decimal string representation of x to b.
func appendDecimal(b []byte, x uint8) []byte {
	// Using this function rather than strconv.AppendUint makes IPv4
	// string building 2x faster.

	if x >= 100 {
		b = append(b, digits[x/100])
	}
	if x >= 10 {
		b = append(b, digits[x/10%10])
	}
	return append(b, digits[x%10])
}

func ConnectToNextHop(relayInfo RelayInfo) {

	var rightConn net.Conn
	var err error

	nextHop := relayInfo.NextHop.string4() + strconv.Itoa(relayInfo.NextHopPort)
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
	if n < 12 {
		log.Fatalf("read %d byte from controller, less than 12 bytes!\n", n)
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
	listen, err := net.Listen("tcp", ListenToControllerAddr)
	if err != nil {
		log.Fatalln("ListenToController: Listen() failed, error = ", err)
		return
	} else {
		log.Println("ListenToController: Listen to controller success!")
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
		log.Fatalf("Listen at %s failed, error = %v\n", address, err)
		return
	} else {
		log.Printf("Listen at %s, wait pre hop to connect us.\n", address)
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

func registerNode() {
	//连接server端
	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	conn, err := grpc.DialContext(ctx, controllerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("grpc.Dial err")
	}
	defer conn.Close()

	c := services.NewControllerClient(conn)
	defer cancel()
	resp, err := c.RegisterNode(ctx, &services.RegisterNodeRequest{Ip: ipStringToInt("localhost")})
	if err != nil {
		log.Printf("failed to call RegisterNode, error = %v\n", err)
		return
	}
	fmt.Printf("register node, get node id = %d\n", resp.NodeId)
}

func ipStringToInt(ip string) int32 {
	if ip == "localhost" {
		ip = "127.0.0.1"
	}
	strs := strings.Split(ip, ".")
	if len(strs) != 4 {
		panic("wrong ip fromat!")
	}
	tmp := make([]byte, 0, 4)
	for i := 0; i < 4; i++ {
		v, err := strconv.Atoi(strs[i])
		if err != nil {
			panic("failed to call strconv.Atoi")
		}
		tmp = append(tmp, byte(v))
	}
	// 对于localhost而言，返回的就是0x100007f
	return int32(binary.LittleEndian.Uint32(tmp))

}
func main() {
	fmt.Printf("%x\n", ipStringToInt("localhost"))
	fmt.Println(int(unsafe.Sizeof(RelayInfo{})))
	registerNode()
	go ListenToController()
	ListenToLeft(ListenToLeftAddr)

}
