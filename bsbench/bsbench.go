package main

import (
	"bytes"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/codingeasygo/bsck"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
)

func usage() {
	fmt.Printf("Usage: bsbench <node|bandwidth|bench> <name> <channel1> <channel2> ... <options>\n")
	flag.PrintDefaults()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("command is not setted\n")
		usage()
		os.Exit(1)
		return
	}
	runtime.GOMAXPROCS(8)
	var log int
	var listen, debug, uri string
	var bufferSize int
	var total, concurrent int64
	flag.StringVar(&debug, "debug", "", "debug listen address")
	flag.IntVar(&bufferSize, "bs", 64, "the buffer size by KB")
	flag.IntVar(&log, "log", bsck.LogLevelError, "the log level")
	switch os.Args[1] {
	case "node":
		flag.StringVar(&listen, "listen", "", "listen address")
		flag.CommandLine.Parse(os.Args[2:])
	case "bandwidth":
		flag.StringVar(&uri, "uri", "", "the uri to bandwith")
		flag.CommandLine.Parse(os.Args[2:])
		if len(uri) < 1 {
			fmt.Printf("-uri is not setted\n")
			usage()
			os.Exit(1)
			return
		}
	case "bench":
		flag.StringVar(&uri, "uri", "", "the uri to bench")
		flag.Int64Var(&total, "total", 10000, "the total count to bench")
		flag.Int64Var(&concurrent, "conc", 100, "the concurrent size to bench")
		flag.CommandLine.Parse(os.Args[2:])
		if len(uri) < 1 {
			fmt.Printf("-uri is not setted\n")
			usage()
			os.Exit(1)
			return
		}
	}
	args := flag.Args()
	if len(args) < 1 {
		fmt.Printf("name is not setted\n")
		usage()
		os.Exit(1)
		return
	}
	config := &bsck.Config{
		Name:   args[0],
		Listen: listen,
		ACL:    map[string]string{".*": ".*"},
		Log:    log,
		Dialer: xmap.M{"std": 1},
		Access: [][]string{{".*", ".*"}},
	}
	for index, remote := range args[1:] {
		config.Channels = append(config.Channels, xmap.M{
			"enable": 1,
			"index":  index,
			"remote": remote,
			"token":  config.Name,
		})
	}
	service := bsck.NewService()
	service.BufferSize = bufferSize * 1024
	service.Config = config
	err := service.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start fail with %v\n", err)
		os.Exit(1)
	}
	if len(debug) > 0 {
		http.HandleFunc("/adm/status", service.Node.StateH)
		go http.ListenAndServe(debug, nil)
	}
	switch os.Args[1] {
	case "node":
		wc := make(chan os.Signal, 1)
		signal.Notify(wc, os.Interrupt, syscall.SIGTERM)
		<-wc
	case "bandwidth":
		Bandwidth(service, uri)
	case "bench":
		Benchmark(service, uri, concurrent, total)
	}
	service.Stop()
}

// BandwidthConn is io.ReadWriteCloser to test bandwidth
type BandwidthConn struct {
	Error      error
	Data       []byte
	ReadCount  int64
	WriteCount int64
	Waiter     sync.WaitGroup
}

// NewBandwidthConn will return new connection
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

// Close will set closed error
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

// Bandwidth will test bandwidth
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

// BenchmarkConn is connection to benchmark test
type BenchmarkConn struct {
	Waiter  sync.WaitGroup
	Send    []byte
	Recv    []byte
	sending []byte
}

// NewBenchmarkConn will return new BenchmarkConn
func NewBenchmarkConn(data []byte) (conn *BenchmarkConn) {
	conn = &BenchmarkConn{
		Waiter:  sync.WaitGroup{},
		Send:    data,
		sending: data,
	}
	conn.Waiter.Add(1)
	return
}

func (b *BenchmarkConn) Read(p []byte) (n int, err error) {
	if len(b.sending) < 1 {
		b.Waiter.Wait()
		err = fmt.Errorf("done")
		return
	}
	n = copy(p, b.sending)
	b.sending = b.sending[n:]
	return
}

func (b *BenchmarkConn) Write(p []byte) (n int, err error) {
	b.Recv = append(b.Recv, p...)
	if len(b.Recv) >= len(b.Send) {
		b.Waiter.Done()
	}
	return
}

// Close will set closed error
func (b *BenchmarkConn) Close() (err error) {
	return
}

func (b *BenchmarkConn) String() string {
	return "BandwidthConn"
}

// Benchmark will do benchmark test to uri
func Benchmark(service *bsck.Service, uri string, concurrent, total int64) (err error) {
	uri = uri + "->tcp://echo"
	waitConn0, waitConn1, _ := xio.Pipe()
	for {
		_, err = service.DialAll(uri, waitConn1, true)
		if err != nil {
			ErrorLog("Benchmark dial to %v fail with %v", uri, err)
			time.Sleep(time.Second)
			continue
		}
		break
	}
	waitConn0.Close()
	//
	InfoLog("Benchmark test is starting with total:%v,concurrent:%v", total, concurrent)
	task := make(chan int64, 10000)
	waiter := sync.WaitGroup{}
	for i := int64(0); i < concurrent; i++ {
		waiter.Add(1)
		go func() {
			for {
				index := <-task
				if index < 0 {
					break
				}
				data := fmt.Sprintf("benchmark-%v", index)
				conn := NewBenchmarkConn([]byte(data))
				_, dialErr := service.DialAll(uri, conn, true)
				if dialErr != nil {
					ErrorLog("Benchmark wait dial to %v fail with %v", uri, dialErr)
					continue
				}
				conn.Waiter.Wait()
				if !bytes.Equal(conn.Send, conn.Recv) {
					ErrorLog("Benchmark test result fail with %v, expect %v", uri, string(conn.Recv), string(conn.Send))
					continue
				}
			}
			waiter.Done()
		}()
	}
	begin := time.Now()
	for i := int64(0); i < total; i++ {
		task <- i
	}
	for i := int64(0); i < concurrent; i++ {
		task <- -1
	}
	waiter.Wait()
	used := time.Since(begin)
	InfoLog("Benchmark test done with total:%v,concurrent:%v,used:%v,avg:%v", total, concurrent, used, used/time.Duration(total))
	return
}
