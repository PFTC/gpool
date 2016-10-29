//This is a example: webserver using the goroutine pool
package main

import (
	"github.com/pftc/gpool"
	"log"
	"net"
	"runtime"
	//"net/http"
)

func main() {
	listener, err := net.Listen("tcp", ":8088")
	if err != nil {
		log.Fatal(err)
	}

	wp, _ := gpool.NewLimit(10000)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("conn fetch error")
			continue
		}
		f := func() {
			conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nfoo"))
			conn.Close()
		}
		wp.Queue(f)
	}
}
