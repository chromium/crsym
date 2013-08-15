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
	"github.com/chromium/crsym/breakpad"
)

// NewCrashKeyInputParser returns an InputParser that connects to a
// AnnotatedFrameService backend. It retrieves the crash report with the given
// ID, and it extracts a stack trace (a string of whitespace-separated
// addresses) from the report. This stack trace is then symbolized using the
// module list provided by the crash report, via the FrameService.
func NewCrashKeyInputParser(service breakpad.AnnotatedFrameService, reportID, key string) InputParser {
	return NewGeneratorInputParser(func(parser *GeneratorInputParser, input string) error {
		frames, err := service.GetAnnotatedFrames(reportID, key)
		if err != nil {
			return err
		}

		for _, frame := range frames {
			parser.EmitStackFrame(0, GIPStackFrame{
				RawAddress: frame.Address,
				Address:    frame.Address,
				Module:     frame.Module,
			})
		}
		return nil
	})
}
