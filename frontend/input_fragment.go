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
	"strings"

	"github.com/chromium/crsym/breakpad"
)

type fragmentInputParser struct {
	module      breakpad.SupplierRequest
	baseAddress uint64
}

// NewFragmentInputParser returns an InputParser that can parse a whitespace-
// separated string of addresses and will symbolize them, returning each frame
// on a new line.
//
// Because the parser cannot derive code module information from the input, all
// the necessary parameters for symbolization must be supplied here.
func NewFragmentInputParser(moduleName, identifier string, baseAddress uint64) InputParser {
	fip := &fragmentInputParser{
		module: breakpad.SupplierRequest{
			ModuleName: moduleName,
			Identifier: identifier,
		},
		baseAddress: baseAddress,
	}
	return NewGeneratorInputParser(func(gip *GeneratorInputParser, input string) error {
		return fip.parseAddresses(gip, input)
	})
}

func (p *fragmentInputParser) parseAddresses(gip *GeneratorInputParser, input string) error {
	addresses := strings.Fields(input)
	for _, address := range addresses {
		absAddress, err := breakpad.ParseAddress(address)
		if err != nil {
			gip.EmitStackFrame(0, GIPStackFrame{Placeholder: address})
		} else {
			gip.EmitStackFrame(0, GIPStackFrame{
				RawAddress: absAddress,
				Address:    absAddress - p.baseAddress,
				Module:     p.module,
			})
		}
	}
	return nil
}
