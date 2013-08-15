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
	Package breakpad supplies two interfaces: Supplier and SymbolTable. The
	SymbolTable has one provided implementation, which parses the Breakpad symbol
	file format, documented here:
		<http://code.google.com/p/google-breakpad/wiki/SymbolFiles>.

	There is no provided Supplier implementation as most clients will likely use
	on-disk files. However, an interface is provided for those that need to RPC
	to a backend to get symbol file data.
*/
package breakpad

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

// SymbolTable provides a way to query information about a code module and to
// lookup symbols by addresses in the module.
type SymbolTable interface {
	// ModuleName returns the debug file name for which this is a symbol table.
	ModuleName() string

	// Identifier returns the unique debug identifier for this module.
	Identifier() string

	// String returns a huamn-friendly representation of the module.
	String() string

	// SymbolForAddress takes a program counter address, relative to the base
	// address of the module, and returns the Symbol to which it relates. If
	// the address is not within the module or a symbol cannot be found, returns
	// nil.
	SymbolForAddress(address uint64) *Symbol
}

// Symbol stores the name of and potentially debug information about a function
// or instruction in a SymbolTable.
type Symbol struct {
	// The function's unmangled name. Never empty.
	Function string

	// The file in which the function was implemented. Can be empty.
	File string
	// The 1-based line at which an instruction occurred. Can be 0 for no line
	// information.
	Line int
}

// FileLine returns the formatted file/line information in a standard way.
func (s *Symbol) FileLine() string {
	if s.File == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", path.Base(s.File), s.Line)
}

// ParseAddress converts a hex string in either 0xABC123 or just ABC123 form
// into an integer.
func ParseAddress(addr string) (uint64, error) {
	if strings.HasPrefix(addr, "0x") {
		addr = addr[2:]
	}
	return strconv.ParseUint(addr, 16, 64)
}
