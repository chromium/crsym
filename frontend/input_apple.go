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

package frontend

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/chromium/crsym/breakpad"
)

const (
	kReportVersion = "Report Version:"

	kEventType = "Event:"

	kBinaryImages = "Binary Images:"

	kSampleAnalysisWritten = "Sample analysis of process"
)

var (
	// Pattern to match a "Binary Images" line. Groups:
	//  1) Base address of the module
	//  2) The module name, as reported by CFBundleName
	//  3) The module's UUID, from LC_UUID load command
	//  4) Path to the binary image
	// Matches:
	// |0x520ce000 - 0x520ceff7 +com.google.Chrome.canary 17.0.959.0 (959.0) <8BC87704-1B47-6F0C-70DE-17F7A99A1E45> /Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary|
	kBinaryImage = regexp.MustCompile(`\s*0x([[:xdigit:]]+)\s*-\s*0x[[:xdigit:]]+\s+\+?([a-zA-Z0-9_\-+.]+) [^<]* <([[:xdigit:]\-]+)> (.*)`)

	// Pattern to match a V9 crash report stack frame. Groups:
	//  1) Portion of the frame to remain untouched
	//  2) Module name, with trailing whitespace
	//  3) Instruction address
	//  4) Symbol information (typically "name + offset")
	// Matches:
	// |4   com.google.Chrome.framework		0x528b225b ChromeMain + 8239323|
	kCrashFrame = regexp.MustCompile(`(\d+[ ]+(.*)\s+0x([[:xdigit:]]+)) ((.*) \+ (.*))`)

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

// AppleInputParser takes an Apple-style crash report and symbolizes it. The
// original input format will remain untouched, but the function names will be
// replaced where symbol data is available.
type AppleInputParser struct {
	// The reportVersion, which influences the parsing of stack frames.
	reportVersion int

	// A map of module names to images.
	modules map[string]binaryImage

	// Input lines.
	lines []string
}

func (p *AppleInputParser) ParseInput(data string) error {
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
		if strings.HasPrefix(line, kBinaryImages) {
			if err := p.parseBinaryImages(i + 1); err != nil {
				return err
			}
		}
	}

	knownVersions := []int{
		6,  // 10.5 and 10.6 crash report.
		7,  // 10.7 sample/hang report.
		9,  // 10.7 crash report.
		10, // 10.8 crash report.
	}
	known := false
	for _, version := range knownVersions {
		if version == p.reportVersion {
			known = true
			break
		}
	}
	if !known {
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

func (p *AppleInputParser) parseBinaryImages(startIndex int) error {
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

func (p *AppleInputParser) RequiredModules() []breakpad.SupplierRequest {
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
func (p *AppleInputParser) FilterModules() bool {
	return true
}

func (p *AppleInputParser) Symbolize(tables []breakpad.SymbolTable) string {
	switch p.reportVersion {
	case 6:
		p.symbolizeCrash(tables)
	case 7:
		p.symbolizeHang(tables)
	case 9:
		p.symbolizeCrash(tables)
	case 10:
		p.symbolizeCrash(tables)
	default:
		panic(fmt.Sprintf("Unknown report version %d", p.reportVersion))
	}
	return strings.Join(p.lines, "\n")
}

func (p *AppleInputParser) mapTables(tables []breakpad.SymbolTable) map[string]breakpad.SymbolTable {
	m := make(map[string]breakpad.SymbolTable)
	for _, table := range tables {
		m[table.ModuleName()] = table
	}
	return m
}

func (p *AppleInputParser) symbolizeCrash(tables []breakpad.SymbolTable) error {
	tableMap := p.mapTables(tables)

	// Go through the report, symbolizing any frames that match the pattern.
	for i, line := range p.lines {
		frame := kCrashFrame.FindStringSubmatch(line)
		if frame == nil {
			// Skip over lines that aren't stack frames.
			continue
		}

		// Get the module based on the name present in the Binary Images
		// section.
		moduleName := strings.TrimSpace(frame[2])
		binaryImage, ok := p.modules[moduleName]
		if !ok {
			continue
		}

		// From the binaryImage, get the SymbolTable.
		table, ok := tableMap[binaryImage.breakpadName()]
		if !ok {
			continue
		}

		address, err := breakpad.ParseAddress(frame[3])
		if err != nil {
			return err
		}

		symbol := table.SymbolForAddress(address - binaryImage.baseAddress)
		if symbol == nil {
			continue
		}

		// Overwrite the input lines.
		p.lines[i] = fmt.Sprintf("%s %s (%s)", frame[1], symbol.Function, symbol.FileLine())
	}
	return nil
}

func (p *AppleInputParser) symbolizeHang(tables []breakpad.SymbolTable) error {
	tableMap := p.mapTables(tables)

	// The p.modules is mapped by bundle ID, so re-map it to be done by breakpad
	// name.
	modules := make(map[string]binaryImage, len(p.modules))
	for _, module := range p.modules {
		modules[module.breakpadName()] = module
	}

	// Iterate over the lines, symbolizing them in-place.
	for i, line := range p.lines {
		frame := kHangFrameV7.FindStringSubmatchIndex(line)
		if frame == nil {
			// Skip over non-frame lines.
			continue
		}

		getSubstring := func(group int) string {
			return line[frame[2*group]:frame[2*group+1]]
		}

		// Get the breakpad name of the module to get the table.
		breakpadName := getSubstring(3)
		table, ok := tableMap[breakpadName]
		if !ok {
			continue
		}

		// Look up the binary image to get its load address.
		binaryImage, ok := modules[breakpadName]
		if !ok {
			continue
		}

		// Get the instruction address to symbolize.
		address, err := breakpad.ParseAddress(getSubstring(5))
		if err != nil {
			return err
		}

		symbol := table.SymbolForAddress(address - binaryImage.baseAddress)
		if symbol == nil {
			continue
		}

		// Fix up the line. The format is such:
		// [beginning of line to symbol] [symbolized function name] [original line until address] [symbolized file/line] [to end of line]
		p.lines[i] = line[0:frame[4]] + symbol.Function + line[frame[5]:frame[10]] + symbol.FileLine() + line[frame[11]:]
	}

	return nil
}
