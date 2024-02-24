package main

import (
	"context"
	"fmt"
	"log"
	"node/services"
	"os"
	"os/signal"
	"time"

	probing "github.com/prometheus-community/pro-bing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func GetCpuUsageRate() {
	for {

		time.Sleep(time.Second * 20)
		fmt.Println("update cpu usage to controller")
	}
}

func GetMemUsageRate() {
	for {

		time.Sleep(time.Second * 20)
		fmt.Println("update memory usage to controller")

	}
}

func pingNode(id int) (int, int) {
	pinger, err := probing.NewPinger("baidu.com")
	if err != nil {
		panic(err)
	}
	// Listen for Ctrl-C.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			pinger.Stop()
		}
	}()

	pinger.Count = 8
	pinger.OnRecv = func(pkt *probing.Packet) {
		fmt.Printf("%d bytes from %s: icmp_seq=%d time=%v\n",
			pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt)
	}
	pinger.OnDuplicateRecv = func(pkt *probing.Packet) {
		fmt.Printf("%d bytes from %s: icmp_seq=%d time=%v ttl=%v (DUP!)\n",
			pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt, pkt.TTL)
	}
	pinger.OnFinish = func(stats *probing.Statistics) {
		fmt.Printf("\n--- %s ping statistics ---\n", stats.Addr)
		fmt.Printf("%d packets transmitted, %d packets received, %v%% packet loss\n",
			stats.PacketsSent, stats.PacketsRecv, stats.PacketLoss)
		fmt.Printf("round-trip min/avg/max/stddev = %v/%v/%v/%v\n",
			stats.MinRtt, stats.AvgRtt, stats.MaxRtt, stats.StdDevRtt)
	}
	fmt.Printf("PING %s (%s):\n", pinger.Addr(), pinger.IPAddr())
	err = pinger.Run()
	if err != nil {
		panic(err)
	}
	s := pinger.Statistics()
	return int(s.AvgRtt.Milliseconds()), int(s.PacketLoss)
}

func GetLinkMetrics() {
	for {
		avgRtt, dropRate := pingNode(2)
		fmt.Printf("get avgRtt = %d, get dropRate = %d", avgRtt, dropRate)

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
		req := &services.UpdateLinkMetricRequest{SrcNodeId: 0, DstNodeId: 1, LinkDropRate: int32(dropRate), LinkLatency: int32(avgRtt)}
		resp, err := c.UpdateLinkMetric(ctx, req)
		if err != nil {
			log.Printf("failed to call UpdateLinkMetric, error = %v\n", err)
			return
		}
		fmt.Printf("call UpdateLinkMetricRequest, get response with status= %d\n", resp.Status)
		time.Sleep(time.Second * 20)
		fmt.Println("update link metrics to controller")

	}
}

func GetEbpfMetrics() {
	for {

		time.Sleep(time.Second * 20)
		fmt.Println("update ebpf metrics to controller")
	}
}

// TODO: 需要想个办法让协程知道何时退出
func GetMetrics() {
	go GetCpuUsageRate()
	fmt.Println("start to get cpu usage rate.")
	go GetMemUsageRate()
	fmt.Println("start to get memory usage rate.")
	go GetLinkMetrics()
	fmt.Println("start to link drop rate rate and link latency")
	go GetEbpfMetrics()
	fmt.Println("start to get all ebpf metrics.")
}
