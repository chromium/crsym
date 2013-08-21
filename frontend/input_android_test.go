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
	"fmt"
	"strings"
	"testing"

	"github.com/chromium/crsym/breakpad"
	"github.com/chromium/crsym/context"
	"github.com/chromium/crsym/testutils"
)

// testModuleInfoServiceAndroid is a stub class that allows us to test just the
// parsing portion of androidInputParser.
type testModuleInfoServiceAndroid struct {
	version string
}

func (t *testModuleInfoServiceAndroid) GetModulesForProduct(ctx context.Context, product, version string) ([]breakpad.SupplierRequest, error) {
	t.version = version
	return []breakpad.SupplierRequest{
		breakpad.SupplierRequest{
			ModuleName: "libchromeview.so",
			Identifier: "1",
		},
	}, nil
}

func TestParseInputAndroid(t *testing.T) {
	// These inputs are valid
	goodInputs := []struct {
		input   string
		version string
	}{
		{"W/google-breakpad(0): 1.2.3.4\n", "1.2.3.4"},
		{"W/google-breakpad(0): 1234\n", "1234"},
		{"W/google-breakpad(0123): 0\n", "0"},
		{"W/google-breakpad(0): 0\n #00  pc 006fbe5a  libchromeview.so\n", "0"},
		{"W/google-breakpad(0): 0\n #00  pc 006fbe5a  libchromeview.so (func)\n", "0"},
		{"W/google-breakpad(0): 0\n #00  xx 006fbe5a  libchromeview.so\n", "0"},
		{"W/google-breakpad(0): 0\n #99  pc 006fbe5a  libchromeview.so\n", "0"},
	}

	var testmod testModuleInfoServiceAndroid

	for _, test := range goodInputs {
		parser := NewAndroidInputParser(context.Background(), &testmod, "")
		if err := parser.ParseInput(test.input); err != nil {
			t.Error("Did not expect error for input: " + test.input)
		}

		if test.version != testmod.version {
			t.Error("Expected version: " + test.version + " for input: " + test.input)
		}
	}

	// These inputs are not valid
	badInputs := []struct {
		input    string
		errorStr string
	}{
		{"W/google-breakpad(0): b7247ee2-5177-40fd-8959-33bc2f793db9\n", "Version number of Chrome"},
		{"W/google-breakpad(0): 1.2.3.4.\n", "Version number of Chrome"},
		{"W/google-breakpad(0): 1234\n #18446744073709551616  pc 006fbe5a  /system/lib/libchromeview.so\n", "frame number"},
	}

	for _, test := range badInputs {
		parser := NewAndroidInputParser(context.Background(), &testmod, "")
		if err := parser.ParseInput(test.input); err == nil {
			t.Error("Expected error for input: " + test.input)
		} else {
			if !strings.Contains(err.Error(), test.errorStr) {
				t.Error("Expected \"" + test.errorStr + "\" as the error")
			}
		}
	}
}

// TestSymbolizeAndroid tests the symbolize function of androidInputParser.  This function
// is almost identical to the TestSymbolize function in input_apple_test.go.
func TestSymbolizeAndroid(t *testing.T) {
	files := []string{
		"android1.txt",
		"android2.txt",
	}

	for _, file := range files {
		var testmod testModuleInfoServiceAndroid

		inputData, err := testutils.ReadSourceFile(testdata(file))
		if err != nil {
			t.Errorf("Failed to read file : " + file)
			continue
		}

		tables := []breakpad.SymbolTable{
			&testTable{name: "libchromeview.so", symbol: "Framework"},
		}

		parser := NewAndroidInputParser(context.Background(), &testmod, "")
		err = parser.ParseInput(string(inputData))
		if err != nil {
			t.Errorf("%s: %s", file, err)
			continue
		}

		// Write the output to a .actual file, which can be used to create a new baseline
		// .expected file by copying it into the testdata/ directory.

		actual := parser.Symbolize(tables)
		actualFileName, actualFile, err := testutils.CreateTempFile(file + ".actual")
		if err != nil {
			t.Errorf("Could not create actual file output: %v", err)
			continue
		}
		fmt.Fprint(actualFile, actual)
		actualFile.Close()

		expectedFileName := testutils.GetSourceFilePath(testdata(file + ".expected"))
		err = testutils.CheckFilesEqual(expectedFileName, actualFileName)
		if err != nil {
			t.Errorf("Input data for %s does not symbolize to expected output", file)
			t.Error(err)
		}
	}
}
