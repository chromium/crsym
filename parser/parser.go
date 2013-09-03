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

package parser

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/chromium/crsym/breakpad"
)

// Parser is the interface that describes the input processing pipeline
// for symbolization requests.
type Parser interface {
	// ParseInput is the first step that accepts raw user input and internalizes
	// it. If successful, returns nil, or an error if unsuccessful and
	// processing should stop.
	ParseInput(data string) error

	// Called after ParseInput to report any modules for which symbol
	// information is needed.
	RequiredModules() []breakpad.SupplierRequest

	// Whether this parser should have its RequiredModules() filtered by the
	// breakpad.Supplier. Needed for if RequiredModules returns additional
	// modules that aren't necessairly needed for symbolization.
	FilterModules() bool

	// Takes the data internalized in ParseInput and symbolizes it using a
	// symbol table and its base address. Returns output acceptable for display
	// to a user.
	//
	// The output of invalid or impossible symbolization is the input, possibly
	// transformed for display of valid output.
	Symbolize(tables []breakpad.SymbolTable) string
}

// GeneratorParser is an Parser whose function is to extract thread
// lists from the input string. The output is then generated in a standard
// format that is different from the input format.
type GeneratorParser struct {
	parseFunc  GIPParseFunc
	threadList gipThreadList
	modules    map[string]breakpad.SupplierRequest
}

// GIPParseFunc is called by the GeneratorParser, which should parse the
// input, calling EmitStackFrame for each frame.
type GIPParseFunc func(parser *GeneratorParser, input string) error

type gipThreadList map[int][]GIPStackFrame

// GIPStackFrame contains all the information needed to symbolize a thread's
// stack frame.
type GIPStackFrame struct {
	RawAddress  uint64                   // The address as it appears in the input.
	Address     uint64                   // The address inside the module.
	Module      breakpad.SupplierRequest // Information about the module, used to fetch symbols.
	Placeholder string                   // A string value to use in case the frame cannot be symbolized.
}

// NewGeneratorParser creates a new GeneratorParser that will process
// input using the specified parseFunc.
func NewGeneratorParser(parseFunc GIPParseFunc) *GeneratorParser {
	return &GeneratorParser{
		parseFunc:  parseFunc,
		threadList: make(gipThreadList),
		modules:    make(map[string]breakpad.SupplierRequest),
	}
}

// EmitStackFrame is called by the GIPParseFunc to append a frame to the stack
// for a given thread. The first time this is called for a given thread, the frame
// will be frame 0.
//
// Threads may be emitted in any order, however stack frames for a given thread
// must be emitted in order.
func (gip *GeneratorParser) EmitStackFrame(thread int, frame GIPStackFrame) {
	gip.threadList[thread] = append(gip.threadList[thread], frame)
	if frame.Placeholder == "" {
		if _, ok := gip.modules[frame.Module.ModuleName]; !ok {
			gip.modules[frame.Module.ModuleName] = frame.Module
		}
	}
}

// Parser implementation:

func (gip *GeneratorParser) ParseInput(data string) error {
	return gip.parseFunc(gip, data)
}

func (gip *GeneratorParser) RequiredModules() []breakpad.SupplierRequest {
	modules := make([]breakpad.SupplierRequest, len(gip.modules))
	i := 0
	for _, m := range gip.modules {
		modules[i] = m
		i++
	}
	return modules
}

func (gip *GeneratorParser) FilterModules() bool {
	return false
}

func (gip *GeneratorParser) Symbolize(tables []breakpad.SymbolTable) string {
	showThreadHeaders := len(gip.threadList) > 1

	// Threads are stored in a map so that they can be emitted out of order,
	// but they should be rendered in-order.
	threadOrder := make([]int, len(gip.threadList))
	i := 0
	for threadId, _ := range gip.threadList {
		threadOrder[i] = threadId
		i++
	}
	sort.Ints(threadOrder)

	// Map the symbol tables by their name.
	tableMap := make(map[string]breakpad.SymbolTable, len(tables))
	for _, table := range tables {
		tableMap[table.ModuleName()] = table
	}

	// Symbolize the output in a standard output format.
	output := new(bytes.Buffer)
	for _, threadId := range threadOrder {
		thread := gip.threadList[threadId]

		if showThreadHeaders {
			fmt.Fprintf(output, "Thread %d\n", threadId)
		}

		for _, frame := range thread {
			var sep, fileLine, function string
			if frame.Placeholder != "" {
				function = frame.Placeholder
			} else {
				// Attempt to look up the symbol information.
				var symbol *breakpad.Symbol
				if table := tableMap[frame.Module.ModuleName]; table != nil {
					symbol = table.SymbolForAddress(frame.Address)
				}

				// Format the address, based on whether there's symbol and
				// file/line information.
				if symbol == nil || symbol.FileLine() == "" {
					sep = "+"
					fileLine = fmt.Sprintf("%#x", frame.Address)
				} else {
					sep = "-"
					fileLine = symbol.FileLine()
				}

				if symbol != nil {
					function = symbol.Function
				}
			}

			fmt.Fprintf(output, "%#08x [%s %s\t %s] %s\n", frame.RawAddress, frame.Module.ModuleName, sep, fileLine, function)
		}
	}

	return output.String()
}
