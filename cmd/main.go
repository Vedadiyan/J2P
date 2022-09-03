package main

import (
	"J2PGo/internal"
	"bytes"
	"os"
	"strings"
)

func main() {
	file, err := os.ReadFile("test.json")
	if err != nil {
		panic(err)
	}
	parser := internal.New(file)
	parsed := parser.Parse("test")
	var buffer bytes.Buffer
	for _, value := range parsed {
		str := strings.Split(value, "\n")
		for _, line := range str {
			if len(line) != 0 {
				buffer.WriteString(line)
				buffer.WriteString("\r\n")
			}
		}
	}

	os.WriteFile("test.proto", buffer.Bytes(), 0)
}
