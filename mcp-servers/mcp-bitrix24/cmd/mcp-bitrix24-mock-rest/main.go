package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/magomedcoder/gen/mcp-servers/mcp-bitrix24/internal/bitrix24mock"
	"github.com/magomedcoder/gen/pkg/mcpsafe"
)

func main() {
	addr := flag.String("listen", "127.0.0.1:8899", "адрес mock Bitrix24 REST")
	flag.Parse()

	srv := bitrix24mock.NewServer()

	log.Printf("Bitrix24 mock REST: http://%s/rest/1/mock-token/<method>", *addr)
	log.Fatal(http.ListenAndServe(*addr, mcpsafe.RecoverPanic("mcp-bitrix24-mock-rest", srv.Handler())))
}
