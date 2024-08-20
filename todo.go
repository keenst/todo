package main

import (
	"fmt"
	"flag"
)

var load bool
var save bool

func init() {
	flag.BoolVar(&load, "load", false, "")
	flag.BoolVar(&save, "save", false, "")
}

func main() {
	flag.Parse()

	if (load) {
		print("should load")
	}

	if (save) {
		print("should save")
	}
}

func print(message string) {
	fmt.Println(message)
}
