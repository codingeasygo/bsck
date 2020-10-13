package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/codingeasygo/bsck"
	"github.com/codingeasygo/util/xmap"
)

func usage() {
	fmt.Printf("Usage: bsbench <node|bandwidth|bench> <name> <channel1> <channel2> ... <options>\n")
}

func main() {
	if len(os.Args) < 4 {
		usage()
		os.Exit(1)
		return
	}
	var configArgs, optionArgs []string
	switch os.Args[1] {
	case "node":
		configArgs = os.Args[2:]
	case "bandwidth":
		if len(os.Args) < 4 {
			usage()
			os.Exit(1)
			return
		}
		configArgs = os.Args[2 : len(os.Args)-1]
		optionArgs = os.Args[len(os.Args)-1:]
	case "bench":

	}
	config := &bsck.Config{
		Name:   configArgs[0],
		Listen: configArgs[1],
		ACL:    map[string]string{".*": ".*"},
		Log:    bsck.LogLevelDebug,
		Dialer: xmap.M{"std": 1},
		Access: [][]string{{".*", ".*"}},
	}
	for index, remote := range configArgs[2:] {
		config.Channels = append(config.Channels, xmap.M{
			"enable": 1,
			"index":  index,
			"remote": remote,
			"token":  config.Name,
		})
	}
	service := bsck.NewService()
	service.Config = config
	err := service.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start fail with %v\n", err)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "node":
		wc := make(chan os.Signal, 1)
		signal.Notify(wc, os.Interrupt, os.Kill)
		<-wc
	case "bandwidth":
		Bandwidth(service, optionArgs[0])
	case "bench":

	}
	service.Stop()
}

//BandwidthConn is io.ReadWriteCloser to test bandwidth
type BandwidthConn struct {
	Error      error
	Data       []byte
	ReadCount  int64
	WriteCount int64
	Waiter     sync.WaitGroup
}

//NewBandwidthConn will return new connection
func NewBandwidthConn(bufferSize int) (conn *BandwidthConn) {
	conn = &BandwidthConn{
		Waiter: sync.WaitGroup{},
		Data:   make([]byte, bufferSize),
	}
	for i := 0; i < bufferSize; i++ {
		conn.Data[i] = byte(rand.Intn(255))
	}
	return
}

func (b *BandwidthConn) Read(p []byte) (n int, err error) {
	n = copy(p, b.Data)
	err = b.Error
	b.ReadCount += int64(n)
	return
}

func (b *BandwidthConn) Write(p []byte) (n int, err error) {
	n = len(p)
	err = b.Error
	b.WriteCount += int64(n)
	return
}

//Close will set closed error
func (b *BandwidthConn) Close() (err error) {
	b.Error = fmt.Errorf("closed")
	b.Waiter.Done()
	return
}

func (b *BandwidthConn) String() string {
	return "BandwidthConn"
}

func bandwithInfo(speed int64) string {
	if speed >= 1024*1024 {
		return fmt.Sprintf("%vMB", speed/1024/1024)
	} else if speed > 1024 {
		return fmt.Sprintf("%vKB", speed/1024)
	} else {
		return fmt.Sprintf("%vB", speed)
	}
}

//Bandwidth will test bandwidth
func Bandwidth(service *bsck.Service, uri string) (err error) {
	uri = uri + "->tcp://echo"
	conn := NewBandwidthConn(service.BufferSize)
	conn.Waiter.Add(1)
	for {
		_, err = service.DialAll(uri, conn, true)
		if err != nil {
			ErrorLog("Bandwidth dial to %v fail with %v", uri, err)
			time.Sleep(time.Second)
			continue
		}
		break
	}
	go func() {
		for {
			readCount, writeCount := conn.ReadCount, conn.WriteCount
			time.Sleep(time.Second)
			readSpeed, writeSpeed := conn.ReadCount-readCount, conn.WriteCount-writeCount
			InfoLog("Bandwidth current is\n\t\tRead:%v/s\t\t\t%v\n\t\tWrite:%v/s\t\t\t%v\n", bandwithInfo(readSpeed), conn.ReadCount, bandwithInfo(writeSpeed), conn.WriteCount)
		}
	}()
	conn.Waiter.Wait()
	InfoLog("Bandwidth is done with %v", conn.Error)
	return
}
