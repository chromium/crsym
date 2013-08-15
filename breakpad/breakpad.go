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

package breakpad

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type breakpadFile struct {
	osname string
	arch   string
	ident  string
	module string

	// Map of FILE records of kFileNumber to kFileName.
	files map[int64]string

	// FUNC records, in sorted order.
	funcs funcList
	// lastFunc is the last FUNC record encountered.
	lastFunc *funcRecord

	// PUBLIC records, in sorted order.
	publics funcList
}

type funcList []funcRecord

type funcRecord struct {
	address uint64
	size    uint64 // Size of the function in bytes.
	name    string
	lines   []lineRecord // List of LINE records in unsorted order.
}

type lineRecord struct {
	address uint64
	size    uint64 // Number of bytes for this line of code.
	line    int
	file    int64
}

// NewBreakpadSymbolTable takes the data of a Breakpad symbol file, parses
// it, and returns a SymbolTable. If the data was malformed or could not be
// parsed, returns an error.
func NewBreakpadSymbolTable(data string) (SymbolTable, error) {
	table := &breakpadFile{
		files: make(map[int64]string),
	}
	err := table.parseBreakpad(data)
	return table, err
}

// breakpad.SymbolTable implementation:

func (b *breakpadFile) ModuleName() string {
	return b.module
}

func (b *breakpadFile) Identifier() string {
	return b.ident
}

func (b *breakpadFile) String() string {
	if b.ident == "" {
		return "unknown"
	}
	return fmt.Sprintf("%s (%s %s) <%s>", b.module, b.osname, b.arch, b.ident)
}

func (b *breakpadFile) SymbolForAddress(address uint64) *Symbol {
	// Perform binary search on the FUNC records.
	low, high := 0, len(b.funcs)
	for low < high {
		mid := low + (high-low)/2
		f := b.funcs[mid]
		if address >= f.address && address < f.address+f.size {
			sym := &Symbol{Function: f.name}
			b.lineAtAddress(address, f, sym)
			return sym
		} else if address > f.address {
			low = mid + 1
		} else {
			high = mid
		}
	}

	// Perform an upper-bound search for |address| and return the PUBLIC
	// record before it, which is the function that contains |address|.
	l := len(b.publics)
	i := sort.Search(l, func(i int) bool {
		return b.publics[i].address > address
	})
	if i <= l && i > 0 {
		return &Symbol{Function: b.publics[i-1].name}
	}

	return nil
}

// lineAtAddress fills in debug file/line information for a Symbol, given an
// instruction address and a funcRecord.
func (b *breakpadFile) lineAtAddress(address uint64, f funcRecord, sym *Symbol) {
	for _, l := range f.lines {
		if address >= l.address && address < l.address+l.size {
			sym.File = b.files[l.file]
			sym.Line = l.line
			return
		}
	}
}

// The different record types.
const (
	kRecordModule = "MODULE"
	kRecordFile   = "FILE"
	kRecordFunc   = "FUNC"
	kRecordPublic = "PUBLIC"
	kRecordStack  = "STACK" // Ignored by this implementation.
	kRecordInfo   = "INFO"  // Ignored by this implementation. Windows, non-standard.
)

// Fields of a MODULE record.
const (
	_           = iota
	kModuleOS   = iota
	kModuleArch = iota
	kModuleID   = iota
	kModuleName = iota
	kModule_Len = iota
)

// Fields of a FILE record.
const (
	_           = iota
	kFileNumber = iota
	kFileName   = iota
	kFile_Len   = iota
)

// Fields of a FUNC record.
const (
	_              = iota
	kFuncAddress   = iota
	kFuncSize      = iota
	kFuncParamSize = iota
	kFuncName      = iota
	kFunc_Len      = iota
)

// Fields of a Line record. NOTE: There is no header for this line.
const (
	kLineAddress    = iota
	kLineSize       = iota
	kLineLine       = iota
	kLineFileNumber = iota
	kLine_Len       = iota
)

// Fields of a PUBLIC record.
const (
	_                = iota
	kPublicAddress   = iota
	kPublicParamSize = iota
	kPublicName      = iota
	kPublic_Len      = iota
)

// parseBreakpad takes an input string of Breakpad symbol file data and parses
// it into an in-memory representation for a SymbolTable object.
func (b *breakpadFile) parseBreakpad(data string) error {
	reader := bufio.NewReader(strings.NewReader(data))
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		line = strings.TrimRight(line, "\n")

		recordType := strings.SplitN(line, " ", 2)[0]
		switch recordType {
		case kRecordModule:
			b.lastFunc = nil
			if err = b.parseModule(line); err != nil {
				return err
			}
		case kRecordFile:
			b.lastFunc = nil
			if err = b.parseFile(line); err != nil {
				return err
			}
		case kRecordFunc:
			b.lastFunc = nil
			if err = b.parseFunc(line); err != nil {
				return err
			}
		case kRecordPublic:
			b.lastFunc = nil
			if err = b.parsePublic(line); err != nil {
				return err
			}
		case kRecordInfo:
			fallthrough
		case kRecordStack:
			b.lastFunc = nil
			continue
		default:
			if b.lastFunc == nil {
				return fmt.Errorf("parse breakpad: unknown line '%s'", line)
			}
			if err = b.parseLine(line); err != nil {
				return err
			}
		}
	}

	sort.Sort(b.funcs)
	sort.Sort(b.publics)

	return nil
}

func (b *breakpadFile) parseModule(line string) error {
	if b.ident != "" {
		return errors.New("parse module: already encountered a MODULE record")
	}

	tokens := strings.SplitN(line, " ", kModule_Len)
	if len(tokens) < kModule_Len {
		return errors.New("parse module: invalid number of tokens")
	}

	b.osname = tokens[kModuleOS]
	b.arch = tokens[kModuleArch]
	b.ident = tokens[kModuleID]
	b.module = tokens[kModuleName]
	return nil
}

func (b *breakpadFile) parseFile(line string) error {
	tokens := strings.SplitN(line, " ", kFile_Len)
	if len(tokens) < kFile_Len {
		return errors.New("parse file: invalid number of tokens")
	}

	num, err := strconv.ParseInt(tokens[kFileNumber], 10, 64)
	if err != nil {
		return fmt.Errorf("parse file number: %v", err)
	}

	if _, ok := b.files[num]; ok {
		return errors.New("parse file: duplicate file line")
	}

	b.files[num] = tokens[kFileName]
	return nil
}

func (b *breakpadFile) parseFunc(line string) error {
	tokens := strings.SplitN(line, " ", kFunc_Len)
	if len(tokens) < kFunc_Len {
		return errors.New("parse func: too few tokens")
	}

	address, err := ParseAddress(tokens[kFuncAddress])
	if err != nil {
		return fmt.Errorf("parse func address: %v", err)
	}
	size, err := ParseAddress(tokens[kFuncSize])
	if err != nil {
		return fmt.Errorf("parse func size: %v", err)
	}

	record := funcRecord{
		address: address,
		size:    size,
		name:    tokens[kFuncName],
	}
	b.funcs = append(b.funcs, record)
	b.lastFunc = &b.funcs[len(b.funcs)-1]
	return nil
}

func (b *breakpadFile) parsePublic(line string) error {
	tokens := strings.SplitN(line, " ", kPublic_Len)
	if len(tokens) < kPublic_Len {
		return errors.New("parse public: too few tokens")
	}

	address, err := ParseAddress(tokens[kPublicAddress])
	if err != nil {
		return fmt.Errorf("parse public address: %v", err)
	}

	record := funcRecord{
		address: address,
		name:    tokens[kPublicName],
	}
	b.publics = append(b.publics, record)
	return nil
}

func (b *breakpadFile) parseLine(line string) error {
	tokens := strings.SplitN(line, " ", kLine_Len)
	if len(tokens) != kLine_Len {
		return errors.New("parse line: invalid number of tokens")
	}
	if b.lastFunc == nil {
		return errors.New("parse line: no corresponding FUNC record")
	}

	address, err := ParseAddress(tokens[kLineAddress])
	if err != nil {
		return fmt.Errorf("parse line address: %v", err)
	}
	size, err := ParseAddress(tokens[kLineSize])
	if err != nil {
		return fmt.Errorf("parse line size: %v", err)
	}
	lineNo, err := strconv.Atoi(tokens[kLineLine])
	if err != nil {
		return fmt.Errorf("parse line line: %v", err)
	}
	file, err := strconv.ParseInt(tokens[kLineFileNumber], 10, 64)
	if err != nil {
		fmt.Errorf("parse line file number: %v", err)
	}

	record := lineRecord{
		address: address,
		size:    size,
		line:    lineNo,
		file:    file,
	}
	b.lastFunc.lines = append(b.lastFunc.lines, record)

	return nil
}

// sort.Interface implementation:

func (l funcList) Len() int {
	return len(l)
}
func (l funcList) Less(i, j int) bool {
	return l[i].address < l[j].address
}
func (l funcList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
