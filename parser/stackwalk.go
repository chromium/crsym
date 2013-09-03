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
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/chromium/crsym/breakpad"
)

type stackwalkParser struct {
	// Maps Breakpad module names to identifiers.
	modules map[string]string
	// Used when parsing the thread list to record which of the above modules
	// are actually used.
	usedModules map[string]bool
	// The crash exception information.
	crashInfo string
	// The key in |threads| indiciating which one crashed.
	crashedThread int
	// The threads of the report, keyed by thread ID to slice of frames.
	threads map[int][]stackwalkFrame
}

// NewStackwalkParser creates an Parser that symbolizes the machine
// format output of `minidump_stackwalk` in breakpad/src/processor/.
func NewStackwalkParser() Parser {
	return &stackwalkParser{
		modules:     make(map[string]string),
		usedModules: make(map[string]bool),
		threads:     make(map[int][]stackwalkFrame),
	}
}

type stackwalkFrame struct {
	module  string
	address uint64
}

// Line prefixes for the machine output of minidump_stackwalk.
const (
	kStackwalkCrash  = "Crash"
	kStackwalkModule = "Module"
)

// Indices into the pipe-separated exception information line.
const (
	kStackwalkCrashException = 1
	kStackwalkCrashAddress   = 2
	kStackwalkCrashThread    = 3
	kStackwalkCrash_Len      = 4
)

// Indicies into pipe-separated lines for kStackwalkModule.
const (
	kStackwalkModuleName       = 1
	kStackwalkModuleIdentifier = 4
	kStackwalkModule_Len       = 8
)

// Indices into the pipe-separated lines of a thread frame.
const (
	kStackwalkFrameThread  = 0
	kStackwalkFrameFrame   = 1
	kStackwalkFrameModule  = 2
	kStackwalkFrameAddress = 6
	kStackwalkFrame_Len    = 7
)

func fieldError(field string, expected, actual int, line string) error {
	return fmt.Errorf("wrong number of fields for a %s, should be %d, got %d, line: %q", field, expected, actual, line)
}

// Parser implementation:

func (p *stackwalkParser) ParseInput(data string) error {
	buf := bytes.NewBufferString(data)

	parsingThreads := false
	for {
		// Read the input string a line at a time.
		line, err := buf.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			} else {
				return err
			}
		}
		line = line[0 : len(line)-1] // Remove \n.

		// There is only one blank line in the input: the separator between the
		// metadata and the thread list.
		if line == "" {
			if !parsingThreads {
				parsingThreads = true
				continue
			} else {
				return errors.New("unexpected blank line: already encountered thread list")
			}
		}

		fields := strings.Split(line, "|")

		if parsingThreads {
			if len(fields) < kStackwalkFrame_Len {
				return fieldError("stack frame", kStackwalkFrame_Len, len(fields), line)
			}
			// Extract the thread ID from the frame and create a new thread
			// slice if it is a new thread.
			threadId, err := strconv.Atoi(fields[kStackwalkFrameThread])
			if err != nil {
				return err
			}

			// Create the frame information.
			address, err := breakpad.ParseAddress(fields[kStackwalkFrameAddress])
			if err != nil {
				return err
			}
			module := fields[kStackwalkFrameModule]
			p.threads[threadId] = append(p.threads[threadId], stackwalkFrame{
				module:  module,
				address: address,
			})
			if module != "" {
				p.usedModules[module] = true
			}
		} else {
			switch fields[0] {
			case kStackwalkCrash:
				if len(fields) < kStackwalkCrash_Len {
					return fieldError("crash line", kStackwalkCrash_Len, len(fields), line)
				}
				p.crashInfo = fields[1] + " @ " + fields[2]
				crashedThread, err := strconv.Atoi(fields[3])
				if err != nil {
					return err
				}
				p.crashedThread = crashedThread
			case kStackwalkModule:
				if len(fields) < kStackwalkModule_Len {
					return fieldError("module", kStackwalkFrame_Len, len(fields), line)
				}
				name := fields[kStackwalkModuleName]
				p.modules[name] = fields[kStackwalkModuleIdentifier]
			}
		}
	}
}

func (p *stackwalkParser) RequiredModules() []breakpad.SupplierRequest {
	requests := make([]breakpad.SupplierRequest, len(p.usedModules))
	i := 0
	for name, _ := range p.usedModules {
		requests[i] = breakpad.SupplierRequest{
			ModuleName: name,
			Identifier: p.modules[name],
		}
		i++
	}
	return requests
}

func (p *stackwalkParser) FilterModules() bool {
	return false
}

func (p *stackwalkParser) Symbolize(tables []breakpad.SymbolTable) string {
	tableMap := make(map[string]breakpad.SymbolTable, len(tables))
	for _, table := range tables {
		tableMap[table.ModuleName()] = table
	}

	const noSymbol = "%d\t [%s\t +\t %#x]\n"

	// The threads of a minidump can be in any order, which is why they are parsed
	// into a map. When symbolizing, put them in numerical order.
	threadOrder := make([]int, len(p.threads))
	i := 0
	for threadId, _ := range p.threads {
		threadOrder[i] = threadId
		i++
	}
	sort.Ints(threadOrder)

	buf := new(bytes.Buffer)
	lastThread := -1
	for _, thread := range threadOrder {
		frames := p.threads[thread]

		// Print the thread header.
		if lastThread < thread {
			lastThread = thread
			if lastThread != 0 {
				buf.WriteByte('\n')
			}
			fmt.Fprintf(buf, "Thread %d", thread)
		}

		// Mark the crashed thread.
		if thread == p.crashedThread {
			fmt.Fprintf(buf, " ( * CRASHED * %s )", p.crashInfo)
		}
		buf.WriteByte('\n')

		// Iterate over the frames of the thread.
		for i, frame := range frames {
			table, ok := tableMap[frame.module]
			if !ok {
				fmt.Fprintf(buf, noSymbol, i, frame.module, frame.address)
				continue
			}

			symbol := table.SymbolForAddress(frame.address)
			if symbol == nil {
				fmt.Fprintf(buf, noSymbol, i, frame.module, frame.address)
				continue
			}

			line := symbol.FileLine()
			if line == "" {
				line = fmt.Sprintf("%#x", frame.address)
			}
			fmt.Fprintf(buf, "%d\t [%s\t -\t %s] %s\n", i, frame.module, line, symbol.Function)
		}
	}
	return buf.String()
}
