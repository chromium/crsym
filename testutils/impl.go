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

package testutils

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

func init() {
	GetSourceFilePath = func(projectRootRelative string) string {
		// Tests will be run in the subdirectory (i.e. in breakpad/ or frontend/),
		// so go up a level to ensure that the subdirectory name is not present twice.
		p, _ := filepath.Abs(path.Join("..", projectRootRelative))
		return p
	}

	ReadSourceFile = func(projectRootRelative string) ([]byte, error) {
		filePath := GetSourceFilePath(projectRootRelative)
		return ioutil.ReadFile(filePath)
	}

	CreateTempFile = func(projectRootRelative string) (string, *os.File, error) {
		filePath := path.Join(os.TempDir(), projectRootRelative)
		f, err := os.Create(filePath)
		return filePath, f, err
	}
}
