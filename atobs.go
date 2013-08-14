/* Copyright 2013 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/


/*
	atobs (Address to Breakpad Symbol) is a drop-in replacement for the atos
	tool on Mac OS X that uses Breakpad symbol files instead of dSYMs.

	atobs only supports the -o and -l flags of atos. Slide addresses and header
	printing are not supported.
*/
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/chromium/crsym/breakpad"
	"github.com/chromium/crsym/frontend"
)

var (
	symbolFile = flag.String("o", "", "The breakpad symbol file, from which symbols will be read")

	baseAddress = flag.String("l", "0x0", "Base/load address of the module")
)

func main() {
	flag.Parse()

	if *symbolFile == "" {
		fatal("Need to specify a symbol file")
	}
	offset, err := breakpad.ParseAddress(*baseAddress)
	if err != nil {
		fatal(err)
	}

	fd, err := os.Open(*symbolFile)
	if err != nil {
		fatal(err)
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		fatal(err)
	}

	table, err := breakpad.NewBreakpadSymbolTable(string(data))
	if err != nil {
		fatal(err)
	}

	input := strings.Join(flag.Args(), " ")

	parser := frontend.NewFragmentInputParser(table.ModuleName(), table.Identifier(), offset)
	if err = parser.ParseInput(input); err != nil {
		fatal(err)
	}

	fmt.Println(parser.Symbolize([]breakpad.SymbolTable{table}))
}

func fatal(msg interface{}) {
	fmt.Println(msg)
	os.Exit(1)
}
