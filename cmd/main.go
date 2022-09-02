package main

import (
	"J2PGo/internal"
	"os"
	"strings"
)

func main() {
	file, err := os.ReadFile("test.json")
	if err != nil {
		panic(err)
	}
	parser := internal.New(file)
	parsed := parser.Parse()
	str := strings.Join(parsed, "\r\n")
	_ = str

}
