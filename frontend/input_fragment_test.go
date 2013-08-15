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
	"testing"

	"github.com/chromium/crsym/breakpad"
)

const kFragmentTestModule = "Fragment Test Module"

func TestRequiredModules(t *testing.T) {
	p := NewFragmentInputParser(kFragmentTestModule, "moduleidentifier", 0xf00bad)
	p.ParseInput("0xabc 0x123 0xdef 0x456")
	reqs := p.RequiredModules()
	if len(reqs) != 1 {
		t.Fatalf("Expected 1 required module, got %d", len(reqs))
	}

	req := reqs[0]

	actual := req.ModuleName
	expected := kFragmentTestModule
	if actual != expected {
		t.Errorf("req.ModuleName expected '%s', got '%s'", expected, actual)
	}

	actual = req.Identifier
	expected = "moduleidentifier"
	if actual != expected {
		t.Errorf("req.Identifier expected '%s', got '%s'", expected, actual)
	}
}

type testSymbolTable struct {
	symbols map[uint64]breakpad.Symbol
}

func (t *testSymbolTable) ModuleName() string {
	return kFragmentTestModule
}
func (t *testSymbolTable) Identifier() string {
	return t.ModuleName()
}
func (t *testSymbolTable) String() string {
	return t.ModuleName()
}
func (t *testSymbolTable) SymbolForAddress(addr uint64) *breakpad.Symbol {
	sym, ok := t.symbols[addr]
	if !ok {
		return nil
	}
	return &sym
}

func TestSymbolize(t *testing.T) {
	const kBaseAddress = 0x666000
	table := &testSymbolTable{map[uint64]breakpad.Symbol{
		0x100:  breakpad.Symbol{Function: "MessageLoop::Run()", File: "message_loop.cc", Line: 40},
		0x150:  breakpad.Symbol{Function: "base::MessagePumpMac::DoDelayedWork()", File: "message_pump_mac.mm", Line: 88},
		0x990:  breakpad.Symbol{Function: "-[BrowserWindowController orderOut:]", File: "browser_window_controller.mm", Line: 222},
		0xFFF5: breakpad.Symbol{Function: "TSMGetCurrentDocument"},
		0xBBAD: breakpad.Symbol{Function: "+[_AClass someMethodSignature:]"},
	}}

	results := map[string]string{
		"0x666100 0x666990 0x675FF5": `0x00666100 [Fragment Test Module -	 message_loop.cc:40] MessageLoop::Run()
0x00666990 [Fragment Test Module -	 browser_window_controller.mm:222] -[BrowserWindowController orderOut:]
0x00675ff5 [Fragment Test Module +	 0xfff5] TSMGetCurrentDocument
`,
		"0x671BAD 0x666150 0x997AbC": `0x00671bad [Fragment Test Module +	 0xbbad] +[_AClass someMethodSignature:]
0x00666150 [Fragment Test Module -	 message_pump_mac.mm:88] base::MessagePumpMac::DoDelayedWork()
0x00997abc [Fragment Test Module +	 0x331abc] 
`,

		"NaN 0xABC123\t0x666990\n\r  LolCatsAreFunny\t\t\tHello \n\r\t\t\n\rKitty\n\n\n0x671BaD": `0x00000000 [ 	 ] NaN
0x00abc123 [Fragment Test Module +	 0x456123] 
0x00666990 [Fragment Test Module -	 browser_window_controller.mm:222] -[BrowserWindowController orderOut:]
0x00000000 [ 	 ] LolCatsAreFunny
0x00000000 [ 	 ] Hello
0x00000000 [ 	 ] Kitty
0x00671bad [Fragment Test Module +	 0xbbad] +[_AClass someMethodSignature:]
`,
	}

	for input, expected := range results {
		p := NewFragmentInputParser(kFragmentTestModule, "Foobad", kBaseAddress)
		err := p.ParseInput(input)
		if err != nil {
			t.Errorf("Error for input '%s': %v", input, err)
		}

		actual := p.Symbolize([]breakpad.SymbolTable{table})
		if actual != expected {
			t.Errorf("Symbolization for input '%s':\nExpected:\n======\n%s\n=====\nActual:\n=====\n%s\n=====", input, expected, actual)
		}
	}
}
