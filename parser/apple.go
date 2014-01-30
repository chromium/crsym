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
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/chromium/crsym/breakpad"
)

type frameModuleType int

const (
	kModuleTypeBundleID frameModuleType = iota
	kModuleTypeBreakpad
)

type appleParser struct {
	// The reportVersion, which determines the value of |lineParser|.
	reportVersion int

	// A map of module names (reverse DNS/bundle ID) to images.
	modules map[string]binaryImage

	// Line buffer array.
	lines []string

	// Function that is called for each element in |lines| to parse out a fragment.
	lineParser func(line string) *appleReportFragment

	// Some reportVersions specify that the stack frame's module name is in reverse DNS/
	// bundle ID format. Others are in path basename/Breakpad module name format. This
	// field stores that type information.
	tableMapType frameModuleType
}

// NewAppleParser creates a Parser for Apple-style crash and hang reports. The
// original input format will remain untouched, but the function names will be
// replaced where symbol data is available.
func NewAppleParser() Parser {
	return &appleParser{}
}

const (
	kReportVersion = "Report Version:"

	kEventType = "Event:"

	kBinaryImages = "Binary Images:"

	kSampleAnalysisWritten = "Sample analysis of process"
)

func (p *appleParser) ParseInput(data string) error {
	p.lines = strings.Split(data, "\n")
	for i, line := range p.lines {
		// "Report Version:" lines in the header.
		if strings.HasPrefix(line, kReportVersion) {
			parts := strings.Split(line, ":")
			if len(parts) != 2 {
				return errors.New("malformed Report Version")
			}
			version, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return fmt.Errorf("malformed Report Version: %v", err)
			}
			p.reportVersion = version
			continue
		}

		// "Binary Images:"
		if strings.HasSuffix(line, kBinaryImages) {
			if err := p.parseBinaryImages(i + 1); err != nil {
				return err
			}
		}
	}

	switch p.reportVersion {
	case 6: // 10.5 and 10.6 crash report.
		p.lineParser = p.symbolizeCrashFragment
		p.tableMapType = kModuleTypeBundleID
	case 7: // 10.7 sample/hang report.
		p.lineParser = p.symbolizeHangFrame
		p.tableMapType = kModuleTypeBreakpad
	case 9: // 10.7 crash report.
		p.lineParser = p.symbolizeCrashFragment
		p.tableMapType = kModuleTypeBundleID
	case 10: // 10.8 crash report.
		p.lineParser = p.symbolizeCrashFragment
		p.tableMapType = kModuleTypeBundleID
	case 11: // 10.9 crash report.
		p.lineParser = p.symbolizeCrashFragment
		p.tableMapType = kModuleTypeBundleID
	case 18: // 10.9 sample report.
		p.lineParser = p.symbolizeHangV18Frame
		p.tableMapType = kModuleTypeBreakpad
	default:
		return fmt.Errorf("unknown Report Version: %d", p.reportVersion)
	}

	return nil
}

type binaryImage struct {
	baseAddress uint64
	name        string
	ident       string
	path        string
}

func (i *binaryImage) breakpadName() string {
	return path.Base(i.path)
}

func (i *binaryImage) breakpadUUID() string {
	const kLen = 33 // Breakpad UUIDs are 33 characters.
	ident := strings.Replace(i.ident, "-", "", -1)
	if l := len(ident); l < kLen {
		ident = ident + strings.Repeat("0", kLen-l)
	}
	return strings.ToUpper(ident)
}

var (
	// Pattern to match a "Binary Images" line. Groups:
	//  1) Base address of the module
	//  2) The module name, as reported by CFBundleName
	//  3) The module's UUID, from LC_UUID load command
	//  4) Path to the binary image
	// Matches:
	// |0x520ce000 - 0x520ceff7 +com.google.Chrome.canary 17.0.959.0 (959.0) <8BC87704-1B47-6F0C-70DE-17F7A99A1E45> /Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary|
	kBinaryImage = regexp.MustCompile(`\s*0x([[:xdigit:]]+)\s*-\s*0x[[:xdigit:]]+\s+\+?([a-zA-Z0-9_\-+.]+) [^<]* <([[:xdigit:]\-]+)> (.*)`)
)

func (p *appleParser) parseBinaryImages(startIndex int) error {
	p.modules = make(map[string]binaryImage)
	for _, line := range p.lines[startIndex:] {
		// Stop at the first line which is blank or starts with "Sample analysis of
		// process [<process ID> written]", which indicates the end of the section.
		if line == "" || strings.HasPrefix(line, kSampleAnalysisWritten) {
			break
		}

		matches := kBinaryImage.FindAllStringSubmatch(line, -1)
		if matches == nil || len(matches) != 1 {
			return fmt.Errorf("invalid binary image: %s", line)
		}

		image := binaryImage{
			name:  matches[0][2],
			ident: matches[0][3],
			path:  matches[0][4],
		}
		var err error
		image.baseAddress, err = breakpad.ParseAddress(matches[0][1])
		if err != nil {
			return fmt.Errorf("parse binary image: %v", err)
		}
		p.modules[image.name] = image
	}
	return nil
}

func (p *appleParser) RequiredModules() []breakpad.SupplierRequest {
	var modules []breakpad.SupplierRequest
	for _, module := range p.modules {
		modules = append(modules, breakpad.SupplierRequest{
			ModuleName: module.breakpadName(),
			Identifier: module.breakpadUUID(),
		})
	}
	return modules
}

// RequiredModules will return a slice of all modules in the Binary Images
// section, so let the supplier filter them.
func (p *appleParser) FilterModules() bool {
	return true
}

type pair [2]int

// appleReportFragment contains information needed to symbolize a stack frame
// from an Apple report. Each member is a pair of indices which should be
// substringed from the line to get the value.
type appleReportFragment struct {
	// The absolute address of the instruction pointer.
	address pair
	// The module name as interpreted by appleParser.tableMapType.
	module pair
	// The unsymbolized function name.
	functionName pair
	// The location to place the file/line information.
	fileNameLocation pair
}

// replacement holds a location (start, end) pair and a string to splice
// into that location.
type replacement struct {
	loc   pair
	value string
}

type replacementList []replacement

// sort.Interface implementation:

func (rl replacementList) Len() int {
	return len(rl)
}
func (rl replacementList) Less(i, j int) bool {
	return rl[i].loc[0] < rl[j].loc[0]
}
func (rl replacementList) Swap(i, j int) {
	rl[i], rl[j] = rl[j], rl[i]
}

func (p *appleParser) Symbolize(tables []breakpad.SymbolTable) string {
	if p.lineParser == nil {
		panic(fmt.Sprintf("Cannot handle report version %d", p.reportVersion))
	}

	tableMap := p.mapTables(tables)

	// The p.modules is mapped by bundle ID, so re-map it to be done by breakpad
	// name.
	var modules map[string]binaryImage
	if p.tableMapType == kModuleTypeBreakpad {
		modules = make(map[string]binaryImage, len(p.modules))
		for _, module := range p.modules {
			modules[module.breakpadName()] = module
		}
	}

	for i, line := range p.lines {
		frag := p.lineParser(line)
		if frag == nil {
			continue
		}

		address, err := breakpad.ParseAddress(line[frag.address[0]:frag.address[1]])
		if err != nil {
			continue
		}

		var binaryImage binaryImage
		moduleName := line[frag.module[0]:frag.module[1]]
		if p.tableMapType == kModuleTypeBreakpad {
			var ok bool
			binaryImage, ok = modules[moduleName]
			if !ok {
				continue
			}
		} else {
			binaryImage = p.modules[moduleName]
		}

		table, ok := tableMap[binaryImage.breakpadName()]
		if !ok {
			continue
		}
		symbol := table.SymbolForAddress(address - binaryImage.baseAddress)

		rl := replacementList{
			{loc: frag.functionName, value: symbol.Function},
			{loc: frag.fileNameLocation, value: symbol.FileLine()},
		}
		sort.Sort(sort.Reverse(rl))
		for _, r := range rl {
			start, end := r.loc[0], r.loc[1]
			p.lines[i] = p.lines[i][:start] + r.value + p.lines[i][end:]
		}
	}

	return strings.Join(p.lines, "\n")
}

// mapTables takes a slice of SymbolTable and transforms it to a map, keyed
// by module name.
func (p *appleParser) mapTables(tables []breakpad.SymbolTable) map[string]breakpad.SymbolTable {
	m := make(map[string]breakpad.SymbolTable)
	for _, table := range tables {
		m[table.ModuleName()] = table
	}
	return m
}

var (
	// Pattern to match a V9 crash report stack frame. Groups:
	//  1) Portion of the frame to remain untouched
	//  2) Module name, with trailing whitespace
	//  3) Instruction address
	//  4) Symbol information (typically "name + offset")
	// Matches:
	// |4   com.google.Chrome.framework		0x528b225b ChromeMain + 8239323|
	kCrashFrame = regexp.MustCompile(`(\d+[ ]+([^\s]+)\s+0x([[:xdigit:]]+)) ((.*) \+ (.*))`)
)

func (p *appleParser) symbolizeCrashFragment(line string) *appleReportFragment {
	frame := kCrashFrame.FindStringSubmatchIndex(line)
	if frame == nil {
		return nil
	}

	return &appleReportFragment{
		module:           pair{frame[4], frame[5]},
		address:          pair{frame[6], frame[7]},
		functionName:     pair{frame[10], frame[11]},
		fileNameLocation: pair{frame[12], frame[13]},
	}
}

var (
	// Pattern to match a V7 hang report stack frame. Groups:
	//	1) Depth and tree markers
	//	2) Symbol name, to be replaced
	//	3) The module name, as reported by the breakpadName.
	//	4) Instruction address
	// Matches:
	// |    +                           ! 2207 RunCurrentEventLoopInMode  (in HIToolbox) + 318  [0x9b9a5723]|
	// |        1069       ChromeMain  (in Google Chrome Framework) + 0  [0x93780]|
	// |   +         1411 ???  (in Google Chrome Framework)  load address 0xbe000 + 0x5de5eb  [0x69c5eb]|
	kFunction    = `\s+\+?\s+([!:|+]\s+)*\d+\s+(.*)  `                             // |   +         1411 ???|
	kLibrary     = `\(in ([^)]*)\)`                                                // |(in Google Chrome Framework)|
	kLoadAddress = `(  load address 0x[[:xdigit:]]+ \+ 0x[[:xdigit:]]+| \+ \d+)  ` // |load address 0xbe000 + 0x5de5eb| or |+ 318|
	kAddress     = `\[(0x[[:xdigit:]]+)\]`                                         // |[0x69c5eb]|
	kHangFrameV7 = regexp.MustCompile(kFunction + kLibrary + kLoadAddress + kAddress)
)

func (p *appleParser) symbolizeHangFrame(line string) *appleReportFragment {
	frame := kHangFrameV7.FindStringSubmatchIndex(line)
	if frame == nil {
		return nil
	}

	fragment := &appleReportFragment{
		address:          pair{frame[10], frame[11]},
		module:           pair{frame[6], frame[7]},
		functionName:     pair{frame[4], frame[5]},
		fileNameLocation: pair{frame[10], frame[11]},
	}
	return fragment
}

var (
	// Pattern to match a V18 hang report stack frame.
	// Matches:
	// |  43 ChromeMain + 41 (Google Chrome Framework) [0x7a159]|
	// |    43 ??? (Google Chrome Framework + 8050864) [0x8248b0]|
	kHangFrameV18 = regexp.MustCompile(`\s+\d+ ((.+)( \+ \d+)?) \((.+) \+ \d+\) \[(0x[[:xdigit:]]+)\]`)
)

func (p *appleParser) symbolizeHangV18Frame(line string) *appleReportFragment {
	frame := kHangFrameV18.FindStringSubmatchIndex(line)
	if frame == nil {
		return nil
	}

	return &appleReportFragment{
		address:          pair{frame[10], frame[11]},
		module:           pair{frame[8], frame[9]},
		functionName:     pair{frame[4], frame[5]},
		fileNameLocation: pair{frame[10], frame[11]},
	}
}
