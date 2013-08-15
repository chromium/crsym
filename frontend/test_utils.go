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
	"path"

	"github.com/chromium/crsym/breakpad"
)

func testdata(f string) string {
	return path.Join("frontend/testdata", f)
}

type testTable struct {
	name    string
	symbol  string
	counter int
}

func (t *testTable) ModuleName() string {
	return t.name
}
func (t *testTable) Identifier() string {
	return t.name
}
func (t *testTable) String() string {
	return t.name
}
func (t *testTable) SymbolForAddress(address uint64) *breakpad.Symbol {
	t.counter++
	return &breakpad.Symbol{
		Function: fmt.Sprintf("%s::Symbol_%d()", t.symbol, t.counter),
		File:     "/path/is/skipped/" + t.name,
		Line:     int(address),
	}
}
