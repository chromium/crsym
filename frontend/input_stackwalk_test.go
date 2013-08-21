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
	"testing"

	"github.com/chromium/crsym/breakpad"
	"github.com/chromium/crsym/testutils"
)

func TestBadInput(t *testing.T) {
	const fieldError = "wrong number of fields for a "
	inputs := []struct {
		input       string
		errorPrefix string
	}{
		{"Crash||", fieldError + "crash"},
		{"Module|com.google.Chrome||", fieldError + "module"},
		{"\n12|2||", fieldError + "stack frame"},
		{"\nInvalidThreadId||||||", "strconv.ParseInt: parsing \"InvalidThreadId"},
		{"\n3||||||InvalidAddress", "strconv.ParseUint: parsing \"InvalidAddress"},
	}

	for i, input := range inputs {
		parser := NewStackwalkInputParser()
		err := parser.ParseInput(input.input + "\n")
		if err == nil {
			t.Errorf("Expected error got nil for input %d: %q", i, input.input)
		} else {
			if !strings.HasPrefix(err.Error(), input.errorPrefix) {
				t.Errorf("Error for %d should have prefix %q, is %q", i, input.errorPrefix, err.Error())
			}
		}
	}
}

func TestSymbolizeStackwalk(t *testing.T) {
	files := []string{
		"stackwalk1.txt",
		"stackwalk2.txt",
	}

	for _, file := range files {
		filePath := testdata(file)
		expectedPath := filePath + ".expected"

		parser := NewStackwalkInputParser()
		inputData, err := testutils.ReadSourceFile(filePath)
		if err != nil {
			t.Errorf("%s: %v", filePath, err)
			continue
		}
		err = parser.ParseInput(string(inputData))
		if err != nil {
			t.Errorf("Error parsing input for %s: %v", file, err)
			continue
		}

		modules := parser.RequiredModules()
		tables := make([]breakpad.SymbolTable, len(modules))
		for i, module := range modules {
			name := module.ModuleName
			// Leave one module a mystery.
			if name == "libSystem.B.dylib" {
				name = "__not found__"
			}
			tables[i] = &testTable{
				name:   name,
				symbol: strings.Replace(module.ModuleName, " ", "_", -1),
			}
		}

		outputData, err := testutils.ReadSourceFile(expectedPath)
		if err != nil {
			t.Errorf("%s: %s", expectedPath, err)
		}

		actual := parser.Symbolize(tables)

		if err := testutils.CheckStringsEqual(string(outputData), actual); err != nil {
			t.Errorf("Input data for %s does not symbolize to expected output", file)
			t.Error(err)
		}
	}
}
