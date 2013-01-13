package main

import (
	"flag"
	"fmt"
	ss "github.com/shadowsocks/shadowsocks-go/shadowsocks"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var config struct {
	server string
	port   int
	passwd string
	core   int
	nconn  int
	nreq   int
	// nsec   int
}

var debug ss.DebugLog

func doOneRequest(client *http.Client, url string, buf []byte) (err error) {
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("GET %s error: %v\n", url, err)
		return err
	}
	for err == nil {
		_, err = resp.Body.Read(buf)
		if debug {
			debug.Println(string(buf))
		}
	}
	if err != io.EOF {
		fmt.Printf("Read %s response error: %v\n", url, err)
	} else {
		err = nil
	}
	return
}

func get(connid int, url, serverAddr string, enctbl *ss.EncryptTable, done chan []time.Duration) {
	reqDone := 0
	reqTime := make([]time.Duration, config.nreq, config.nreq)
	defer func() {
		done <- reqTime[:reqDone]
	}()
	tr := &http.Transport{
		Dial: func(net, addr string) (c net.Conn, err error) {
			return ss.Dial(addr, serverAddr, enctbl)
		},
	}

	buf := make([]byte, 8192)
	client := &http.Client{Transport: tr}
	for ; reqDone < config.nreq; reqDone++ {
		start := time.Now()
		if err := doOneRequest(client, url, buf); err != nil {
			return
		}
		reqTime[reqDone] = time.Now().Sub(start)

		if (reqDone+1)%1000 == 0 {
			fmt.Printf("conn %d finished %d get request\n", connid, reqDone+1)
		}
	}
}

func main() {
	flag.StringVar(&config.server, "s", "127.0.0.1", "server:port")
	flag.IntVar(&config.port, "p", 0, "server:port")
	flag.IntVar(&config.core, "core", 1, "number of CPU cores to use")
	flag.StringVar(&config.passwd, "k", "", "password")
	flag.IntVar(&config.nconn, "nc", 1, "number of connection to server")
	flag.IntVar(&config.nreq, "nr", 1, "number of request for each connection")
	// flag.IntVar(&config.nsec, "ns", 0, "run how many seconds for each connection")
	flag.BoolVar((*bool)(&debug), "d", false, "print http response body for debugging")

	flag.Parse()

	if config.server == "" || config.port == 0 || config.passwd == "" || len(flag.Args()) != 1 {
		fmt.Printf("Usage: %s -s <server> -p <port> -k <password> <url>\n", os.Args[0])
		os.Exit(1)
	}

	runtime.GOMAXPROCS(config.core)
	url := flag.Arg(0)
	if !strings.HasPrefix(url, "http://") {
		url = "http://" + url
	}

	enctbl := ss.GetTable(config.passwd)
	serverAddr := net.JoinHostPort(config.server, strconv.Itoa(config.port))

	done := make(chan []time.Duration)
	for i := 1; i <= config.nconn; i++ {
		go get(i, url, serverAddr, enctbl, done)
	}

	// collect request finish time
	reqTime := make([]int64, config.nconn*config.nreq)
	reqDone := 0
	for i := 1; i <= config.nconn; i++ {
		rt := <-done
		for _, t := range rt {
			reqTime[reqDone] = int64(t)
			reqDone++
		}
	}

	fmt.Println("number of total requests:", config.nconn*config.nreq)
	fmt.Println("number of finished requests:", reqDone)
	if reqDone == 0 {
		return
	}

	// calculate average an standard deviation
	reqTime = reqTime[:reqDone]
	var sum int64
	for _, d := range reqTime {
		sum += d
	}
	avg := float64(sum) / float64(reqDone)

	varSum := float64(0)
	for _, d := range reqTime {
		di := math.Abs(float64(d) - avg)
		di *= di
		varSum += di
	}
	stddev := math.Sqrt(varSum / float64(reqDone))
	fmt.Println("\ntotal time used:", time.Duration(sum))
	fmt.Println("average time per request:", time.Duration(avg))
	fmt.Println("standard deviation:", time.Duration(stddev))
}
