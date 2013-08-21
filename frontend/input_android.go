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
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/chromium/crsym/breakpad"
	"github.com/chromium/crsym/context"
)

// androidFrame comes from parsing stack trace in the logcat.
type androidFrame struct {
	module      string
	address     uint64
	frameNumber uint
	symbol      string
}

type androidInputParser struct {
	context context.Context

	// The breakpad service we use to query the module info.
	service breakpad.ModuleInfoService

	// Use the GeneratorInputParser to format the output.
	genInputParser *GeneratorInputParser

	// The version of the android chrome build.
	version string
}

// NewAndroidInputParse creates an InputParser that symbolizes the log of the
// android chrome stack trace.  Only works when version number of the build is
// included in the log (i.e. only for Official Release builds).
func NewAndroidInputParser(ctx context.Context, service breakpad.ModuleInfoService, version string) InputParser {
	return &androidInputParser{
		service: service,
		version: version,
		context: ctx,
	}
}

// ParseInput parses the android debug log for frame information and for android
// chrome module version..
func (p *androidInputParser) ParseInput(data string) error {
	buf := bytes.NewBufferString(data)

	lines := make([]string, 0)

	for {
		// Read the input string a line at a time.
		line, err := buf.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}

		if line == "" {
			break
		} else if line[len(line)-1] == '\n' {
			line = line[0 : len(line)-1] // Remove \n.
		}

		lines = append(lines, line)
	}

	var err error
	if p.genInputParser, err = p.buildGenInputParser(lines); err == nil {
		return p.genInputParser.ParseInput("")
	} else {
		return err
	}
}

// retrieveChromeModule retrives the chrome module info given a version of this build
// of android chrome.
func (p *androidInputParser) retrieveChromeModule(version string) (breakpad.SupplierRequest, error) {
	modules, err := p.service.GetModulesForProduct(p.context, "Chrome_Android", version)
	const modErrorStr = "Failed to retrieve module for Chrome_Android (%s) from the crash server: %v"
	var retmodule breakpad.SupplierRequest

	if err != nil || modules == nil || len(modules) == 0 {
		if err != nil {
			return retmodule, fmt.Errorf(modErrorStr, version, err)
		} else {
			return retmodule, fmt.Errorf(modErrorStr, version, "no modules returned")
		}
	}

	for _, module := range modules {
		if module.ModuleName == "libchromeview.so" {
			retmodule = module
			break
		}
	}

	if retmodule.ModuleName == "" {
		return retmodule, fmt.Errorf(modErrorStr, version, "empty module name")
	}

	return retmodule, nil
}

// buildGenInputParser performs two steps: 1) parse stack frames from the given input;
// 2) parse out the build version number, which we use to locate a module n the crash
// server.   The parser is derived from clank/tools/stack_core.py.  Once these two steps
// have been completed, this function returns a GeneratorInputParser, which encapsultes
// the infor parsed in these two steps and help to format the output in Symbolize.
func (p *androidInputParser) buildGenInputParser(lines []string) (*GeneratorInputParser, error) {
	// An example of a line of logcat frame:
	// "0I/DEBUG   ( 2636):     #23  pc 0002b5ec  /system/lib/libdvm.so (dvmInterpret(Thread*, Method const*, JValue*)+184)"
	frameLine := regexp.MustCompile("(.*)\\#([0-9]+)[ \t]+(..)[ \t]+([0-9a-f]{8})[ \t]+([^\r\n \t]*)( \\((.*)\\))?")
	// An example of the version number (format 0):
	// "W/google-breakpad(27887): 27.0.1453.105".
	version0Line := regexp.MustCompile("google\\-breakpad(?:\\([0-9]+\\))*: (([0-9]+\\.)+[0-9]+)$")
	// An example of the version number (format 1):
	// "W/google-breakpad(27887): 1453106".
	version1Line := regexp.MustCompile("google\\-breakpad(?:\\([0-9]+\\))*: (([0-9]+\\.)*[0-9]+)$")

	// Keep track of the android chrome version for crash server look-up.
	var version string

	// Keep track of the frames we read in the input.
	frames := make([]androidFrame, 0, len(lines))

	for _, line := range lines {
		// Parse out the version number of this android chrome build.
		if version0Line.MatchString(line) {
			match := version0Line.FindStringSubmatch(line)
			version = match[1]
		} else if version1Line.MatchString(line) && version == "" {
			match := version1Line.FindStringSubmatch(line)
			version = match[1]
		} else if frameLine.MatchString(line) {
			// Parse out a single frame.
			match := frameLine.FindStringSubmatch(line)

			if fnum, err := strconv.ParseUint(match[2], 10, 0); err == nil {
				// ParseAddress cannot fail if the regular expression passes
				addr, _ := breakpad.ParseAddress(match[4])
				frames = append(frames, androidFrame{
					module:      match[5],
					address:     addr,
					frameNumber: uint(fnum),
					symbol:      match[7],
				})
			} else {
				return nil, fmt.Errorf("Failed to parse the frame number %s in line: %s", match[2], line)
			}
		}
	}

	// If a version was given as manual input.  The manual version number supersedes the version in the log.
	if p.version != "" {
		version = p.version
	}

	// Check here to see we found the version number in the log.
	if version == "" {
		return nil, errors.New("Version number of Chrome was not found.")
	}

	// Use the version number to retrieve the chrome module (libchromeview.so).
	if chromeViewModule, err := p.retrieveChromeModule(version); err == nil {
		// Create a GeneratorInputParser.  For every libchromeview symbol, we emit a proper stack frame.
		// For other frames, we store the given module and symbol name as the place holder; they will
		// show up in the final output.
		retparser := NewGeneratorInputParser(func(parser *GeneratorInputParser, input string) error {
			for _, frame := range frames {
				if strings.HasSuffix(frame.module, "libchromeview.so") {
					parser.EmitStackFrame(0, GIPStackFrame{
						RawAddress: frame.address,
						Address:    frame.address,
						Module:     chromeViewModule,
					})
				} else {
					parser.EmitStackFrame(0, GIPStackFrame{
						RawAddress:  frame.address,
						Address:     frame.address,
						Placeholder: "[" + frame.module + "] " + frame.symbol,
					})
				}
			}
			return nil
		})

		return retparser, nil
	} else {
		return nil, err
	}
}

// RequiredModules cannot directly delegate to GeneratorInputParser because it comes
// back with an empty request, which crashes in http.go.  This is likely due to the
// fact that we do not have modules for every symbol.
func (p *androidInputParser) RequiredModules() []breakpad.SupplierRequest {
	reqs := p.genInputParser.RequiredModules()
	retReqs := make([]breakpad.SupplierRequest, 0)

	for _, r := range reqs {
		if r.ModuleName != "" && r.Identifier != "" {
			retReqs = append(retReqs, r)
		}
	}

	return retReqs
}

// FilterModules delegates to GeneratorInputParser, which returns false.
func (p *androidInputParser) FilterModules() bool {
	return p.genInputParser.FilterModules()
}

// Symbolize delegates to GeneratorInputParser.
func (p *androidInputParser) Symbolize(tables []breakpad.SymbolTable) string {
	return p.genInputParser.Symbolize(tables)
}
